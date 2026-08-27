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

// sdkRenderer renders by calling the helm Go library directly - no helm
// binary is required on the host.
type sdkRenderer struct{}

func newSDKRenderer() Renderer { return sdkRenderer{} }

func (sdkRenderer) Name() string { return "helm library" }

func (sdkRenderer) Render(cfg *types.HelmChart, _ generator, workDir string) (string, error) {
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
	client.RepoURL = cfg.RepoUrl
	client.Version = cfg.Version
	client.Username = username(cfg)
	client.Password = password(cfg)

	chartPath, err := client.LocateChart(cfg.Name, settings)
	if err != nil {
		return "", fmt.Errorf("locating chart %q failed error: %w", cfg.Name, err)
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
