package golden

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files with the current output")

// binaryPath builds krmgen once for the whole test run and returns the path.
// Golden scenarios run the real binary as a subprocess: that is the only way
// to observe the true stdout/stderr split and exit codes the specification
// guarantees, and it keeps a fatal path from killing the test binary.
//
// The build is behind a sync.Once - there are a dozen scenarios and rebuilding
// per scenario would dominate the suite's runtime.
var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

func binaryPath(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "krmgen-golden")
		if err != nil {
			buildErr = err
			return
		}
		bin := filepath.Join(dir, "krmgen")
		build := exec.Command("go", "build", "-o", bin, ".")
		build.Dir = repoRoot(t)
		if out, cmdErr := build.CombinedOutput(); cmdErr != nil {
			buildErr = fmt.Errorf("building krmgen: %w\n%s", cmdErr, out)
			return
		}
		builtBin = bin
	})
	if buildErr != nil {
		t.Fatalf("%v", buildErr)
	}
	return builtBin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// chartRepo packages the demo chart into a temporary directory, serves it as
// a plain HTTP chart repository, and returns its base URL. No network is used.
//
// A local chart directory would be simpler, but krmgen cannot address one:
// newGenerator accepts only oci:// and http(s):// repositories.
func chartRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	pkg := exec.Command("helm", "package", filepath.Join(repoRoot(t), "test", "golden", "charts", "demo"), "-d", dir)
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

type result struct {
	stdout   string
	stderr   string
	exitCode int
}

// runScenario copies the fixture to a temp directory, points it at a local
// chart repository, and runs krmgen against it.
func runScenario(t *testing.T, name string) result {
	t.Helper()

	fixture := filepath.Join("fixtures", name)
	workDir := filepath.Join(t.TempDir(), name)
	if err := os.CopyFS(workDir, os.DirFS(fixture)); err != nil {
		t.Fatalf("copying fixture %s: %v", name, err)
	}
	// golden.yaml is the expectation, not an input
	_ = os.Remove(filepath.Join(workDir, "golden.yaml"))

	cmd := exec.Command(binaryPath(t), "generate", workDir)
	cmd.Env = append(os.Environ(), "ARGOCD_ENV_CHART_REPO="+chartRepo(t))

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return result{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCodeOf(t, err)}
}

// exitCodeOf turns cmd.Run's error into an exit code, failing the test on
// anything that is not a clean non-zero exit (a missing binary, for instance).
func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running krmgen: %v", err)
	}
	return exitErr.ExitCode()
}

// assertGolden compares output against the stored file, or rewrites it with
// -update. A diff here means the product's output changed: read it and decide
// whether that was intended before regenerating.
func assertGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("fixtures", name, "golden.yaml")

	if *update {
		if err := os.WriteFile(path, []byte(got), 0600); err != nil {
			t.Fatalf("updating golden %s: %v", path, err)
		}
		t.Logf("golden updated: %s", path)
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading golden %s: %v (run with -update to create it)", path, err)
	}
	if got != string(want) {
		t.Errorf("output does not match %s\n%s", path, diff(string(want), got))
	}
}

func diff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	var b strings.Builder
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			fmt.Fprintf(&b, "line %d:\n  want: %q\n  got:  %q\n", i+1, w, g)
		}
	}
	return b.String()
}

func TestGolden_HelmOnly(t *testing.T) {
	res := runScenario(t, "helm-only")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "helm-only", res.stdout)
}
