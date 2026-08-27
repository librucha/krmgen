package helm

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/cli"

	types "github.com/librucha/krmgen/internal"
)

// localChartRepo packages the demo chart used by the golden suite into a
// temp directory, serves it as a plain HTTP chart repository, and returns
// its base URL. No network is used - mirrors test/golden/harness_test.go's
// chartRepo helper.
func localChartRepo(t *testing.T) string {
	t.Helper()
	return localChartRepoFor(t, "demo")
}

// localChartRepoFor is localChartRepo generalised to any chart under
// test/golden/charts.
func localChartRepoFor(t *testing.T, chartName string) string {
	t.Helper()
	dir := t.TempDir()

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", "test", "golden", "charts", chartName))
	if err != nil {
		t.Fatal(err)
	}

	pkg := exec.Command("helm", "package", repoRoot, "-d", dir)
	if out, err := pkg.CombinedOutput(); err != nil {
		t.Fatalf("helm package failed: %v\n%s", err, out)
	}
	index := exec.Command("helm", "repo", "index", dir)
	if out, err := index.CombinedOutput(); err != nil {
		t.Fatalf("helm repo index failed: %v\n%s", err, out)
	}

	server := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(server.Close)
	return server.URL
}

func TestSDKRendererName(t *testing.T) {
	if got := newSDKRenderer().Name(); got != "helm library" {
		t.Errorf("Name() = %q, want %q", got, "helm library")
	}
}

func TestSDKRendererMatchesTheBinary(t *testing.T) {
	repoURL := localChartRepo(t)

	// namespace: "" is the parity case that matters here - Namespace is
	// optional on types.HelmChart. The binary renderer omits --namespace
	// entirely and lets helm fall back to settings.Namespace() ("default");
	// the SDK renderer used to assign the empty string straight to
	// client.Namespace and render an empty namespace instead. No golden
	// fixture catches this - every one of them sets namespace explicitly -
	// so it has to be pinned here.
	tests := []struct {
		name      string
		namespace string
	}{
		{name: "namespace set", namespace: "default"},
		{name: "namespace omitted", namespace: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &types.HelmChart{
				Name: "demo", RepoUrl: repoURL, ReleaseName: "rel",
				Version: "0.1.0", Namespace: tt.namespace, IgnoreCredentials: true,
			}
			g, err := newGenerator(cfg)
			if err != nil {
				t.Fatal(err)
			}

			viaBinary, err := newBinaryRenderer().Render(cfg, g, t.TempDir())
			if err != nil {
				t.Fatalf("binary renderer: %v", err)
			}
			viaSDK, err := newSDKRenderer().Render(cfg, g, t.TempDir())
			if err != nil {
				t.Fatalf("sdk renderer: %v", err)
			}
			if viaBinary != viaSDK {
				t.Errorf("renderers disagree\nbinary:\n%s\nsdk:\n%s", viaBinary, viaSDK)
			}
		})
	}
}

// TestSDKRendererMatchesHooksGolden guards the loop over rel.Hooks in
// Render: taking rel.Manifest alone silently drops every hook, and no other
// test in this repo would notice - no chart under test/golden/charts carries
// a helm.sh/hook annotation except this one.
//
// The golden it compares against was captured through the binary path - back
// when that was still the default, before selectRenderer was flipped - by
// building krmgen and
// running `generate` against fixtures/helm-hooks by hand - not with
// `go test -update`. That is what makes it evidence: this test asks the SDK
// renderer to reproduce a rendering it took no part in producing.
func TestSDKRendererMatchesHooksGolden(t *testing.T) {
	repoURL := localChartRepoFor(t, "hooked")

	// Mirrors test/golden/fixtures/helm-hooks/krmgen.yaml.
	cfg := &types.HelmChart{
		Name: "hooked", RepoUrl: repoURL, ReleaseName: "rel",
		Version: "0.1.0", Namespace: "default", IgnoreCredentials: true,
	}
	g, err := newGenerator(cfg)
	if err != nil {
		t.Fatal(err)
	}

	viaSDK, err := newSDKRenderer().Render(cfg, g, t.TempDir())
	if err != nil {
		t.Fatalf("sdk renderer: %v", err)
	}

	goldenPath := filepath.Join("..", "..", "test", "golden", "fixtures", "helm-hooks", "golden.yaml")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("reading golden %s: %v", goldenPath, err)
	}

	// The golden was captured as the stdout of `krmgen generate`, which is
	// this same renderer output passed through fmt.Println - i.e. one extra
	// trailing newline on top of what Render returns.
	want := strings.TrimSuffix(string(golden), "\n")
	if viaSDK != want {
		t.Errorf("sdk renderer does not reproduce the binary-captured golden %s\nwant:\n%s\ngot:\n%s", goldenPath, want, viaSDK)
	}
}

// TestSDKRendererOCI_UsesRegistryClientNotRepoURL pins the fix this task
// makes: for an oci:// chart, Render must leave client.RepoURL empty and
// pass cfg.RepoUrl itself as the chart name (mirroring what
// ociHelmGenerator.addRepoArgs hands the binary renderer - see the comment
// in renderer_sdk.go). Before the fix, RepoURL was set to the oci:// URL,
// which routes helm's LocateChart through repo.FindChartInRepoURL - an HTTP
// chart-repo index lookup that does not understand oci:// at all.
//
// This stays hermetic by overriding the locateChart seam
// (internal/helm/renderer_sdk.go) with a stub that hands back a chart
// packaged locally by `helm package` - no registry, real or fake, is ever
// contacted.
func TestSDKRendererOCI_UsesRegistryClientNotRepoURL(t *testing.T) {
	chartSrc, err := filepath.Abs(filepath.Join("..", "..", "test", "golden", "charts", "demo"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pkg := exec.Command("helm", "package", chartSrc, "-d", dir)
	if out, err := pkg.CombinedOutput(); err != nil {
		t.Fatalf("helm package failed: %v\n%s", err, out)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected exactly one packaged chart in %s, got %v (err %v)", dir, entries, err)
	}
	chartPath := filepath.Join(dir, entries[0].Name())

	cfg := &types.HelmChart{
		Name: "demo", RepoUrl: "oci://registry.example.com/charts", ReleaseName: "rel",
		Version: "0.1.0", Namespace: "default", IgnoreCredentials: true,
	}
	g, err := newGenerator(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var gotName, gotRepoURL string
	original := locateChart
	t.Cleanup(func() { locateChart = original })
	locateChart = func(opts *action.ChartPathOptions, name string, _ *cli.EnvSettings) (string, error) {
		gotName = name
		gotRepoURL = opts.RepoURL
		return chartPath, nil
	}

	out, err := newSDKRenderer().Render(cfg, g, t.TempDir())
	if err != nil {
		t.Fatalf("sdk renderer: %v", err)
	}

	if gotRepoURL != "" {
		t.Errorf("RepoURL = %q, want empty for an oci:// chart", gotRepoURL)
	}
	if gotName != cfg.RepoUrl {
		t.Errorf("chart reference passed to LocateChart = %q, want %q (cfg.RepoUrl)", gotName, cfg.RepoUrl)
	}
	if !strings.Contains(out, "kind:") {
		t.Errorf("rendered output does not look like YAML: %q", out)
	}
}
