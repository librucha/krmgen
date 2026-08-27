package helm

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/cli/values"
	"helm.sh/helm/v4/pkg/getter"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"

	types "github.com/librucha/krmgen/internal"
)

// locateChart is a seam over (*action.ChartPathOptions).LocateChart: a
// reviewer noted that Render otherwise calls LocateChart on a concrete
// action.Install with no injection point, so nothing structurally stops a
// future test from silently going live against a real chart repository or
// registry. Tests replace this var to guarantee that can never happen.
var locateChart = func(opts *action.ChartPathOptions, name string, settings *cli.EnvSettings) (string, error) {
	return opts.LocateChart(name, settings)
}

// sdkRenderer renders by calling the helm Go library directly - no helm
// binary is required on the host.
type sdkRenderer struct{}

func newSDKRenderer() Renderer { return sdkRenderer{} }

func (sdkRenderer) Name() string { return "helm library" }

func (sdkRenderer) Render(cfg *types.HelmChart, g generator, workDir string) (string, error) {
	settings := cli.New()

	// DryRunClient never touches a kubeconfig or a cluster, so an empty
	// Configuration is enough - verified by spike before this phase started.
	client := action.NewInstall(new(action.Configuration))
	client.DryRunStrategy = action.DryRunClient
	client.ReleaseName = cfg.ReleaseName
	client.Replace = true // skip the name check, as `helm template` does
	client.IncludeCRDs = true
	client.Namespace = cfg.Namespace
	if client.Namespace == "" {
		// cfg.Namespace is optional (internal/types.go). The binary renderer
		// simply omits --namespace in this case and lets helm fall back to
		// settings.Namespace() (defaults to "default") - mirror that here,
		// or an unset namespace renders as an empty string instead.
		client.Namespace = settings.Namespace()
	}
	client.Version = cfg.Version
	client.Username = username(cfg)
	client.Password = password(cfg)

	// oci:// charts are addressed directly - cfg.RepoUrl is already the
	// complete chart reference (host, repository path and chart name in
	// one), unlike an HTTP(S) chart repo where RepoUrl names only the
	// repository and cfg.Name picks the chart out of it. This mirrors the
	// binary renderer: ociHelmGenerator.addRepoArgs (oci-generator.go) hands
	// `helm template` cfg.RepoUrl as its sole positional CHART argument and
	// never appends cfg.Name either - confirmed against the real
	// oci-public golden scenario, where appending cfg.Name (as
	// ociHelmGenerator.chartId() does, for the unrelated purpose of
	// deriving the login host in chartIdShort) doubles the last path
	// segment and 404s.
	//
	// RepoURL must stay empty for these: ChartPathOptions.LocateChart
	// (helm.sh/helm/v4@v4.2.4, pkg/action/install.go) treats a non-empty
	// RepoURL as an HTTP chart-repo index lookup (repo.FindChartInRepoURL),
	// which does not understand oci:// at all. Passing cfg.RepoUrl as the
	// name instead, with RepoURL left unset, is what routes LocateChart
	// through its OCI/registry-client path.
	chartRef := cfg.Name
	if oci, isOCI := g.(ociHelmGenerator); isOCI {
		registryClient, err := oci.registryClient(settings)
		if err != nil {
			return "", fmt.Errorf("building registry client for chart %q failed error: %w", cfg.Name, err)
		}
		client.SetRegistryClient(registryClient)
		chartRef = cfg.RepoUrl
	} else {
		client.RepoURL = cfg.RepoUrl
	}

	chartPath, err := locateChart(&client.ChartPathOptions, chartRef, settings)
	if err != nil {
		return "", fmt.Errorf("locating chart %q failed error: %w", chartRef, err)
	}
	loaded, err := loader.Load(chartPath)
	if err != nil {
		return "", fmt.Errorf("loading chart %q failed error: %w", chartPath, err)
	}

	files, err := valueFiles(cfg, workDir)
	if err != nil {
		return "", fmt.Errorf("resolving values for chart %q failed error: %w", cfg.Name, err)
	}
	vals, err := (&values.Options{ValueFiles: files}).MergeValues(getter.All(settings))
	if err != nil {
		return "", fmt.Errorf("merging values failed error: %w", err)
	}

	result, err := client.RunWithContext(context.Background(), loaded, vals)
	if err != nil {
		return "", fmt.Errorf("rendering chart %q failed error: %w", cfg.Name, err)
	}

	// helm does not export a converter for this: pkg/cmd/root.go keeps
	// releaserToV1Release unexported, so we repeat its type switch. This is
	// the first thing that breaks on a helm upgrade.
	var rel *releasev1.Release
	switch r := result.(type) {
	case *releasev1.Release:
		rel = r
	case releasev1.Release:
		rel = &r
	case nil:
		rel = nil
	default:
		return "", fmt.Errorf("unexpected release type %T from helm", result)
	}
	if rel == nil {
		return "", fmt.Errorf("helm returned no release for chart %q", cfg.Name)
	}

	// `helm template` writes the trimmed manifest and THEN every hook, each
	// behind its own "# Source:" header. Taking rel.Manifest alone drops the
	// hooks and the goldens diverge.
	var out bytes.Buffer
	fmt.Fprintln(&out, strings.TrimSpace(rel.Manifest))
	for _, h := range rel.Hooks {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", h.Path, h.Manifest)
	}
	return out.String(), nil
}
