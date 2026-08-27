package helm

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"testing"

	types "github.com/librucha/krmgen/internal"
)

// localChartRepo packages the demo chart used by the golden suite into a
// temp directory, serves it as a plain HTTP chart repository, and returns
// its base URL. No network is used - mirrors test/golden/harness_test.go's
// chartRepo helper.
func localChartRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repoRoot, err := filepath.Abs(filepath.Join("..", "..", "test", "golden", "charts", "demo"))
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

	cfg := &types.HelmChart{
		Name: "demo", RepoUrl: repoURL, ReleaseName: "rel",
		Version: "0.1.0", Namespace: "default", IgnoreCredentials: true,
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
}
