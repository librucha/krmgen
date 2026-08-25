package golden

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
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

// minimalEnv returns the smallest environment a subprocess needs to find its
// tools and write temp files: PATH, HOME and TMPDIR (the latter only if set;
// most platforms default it internally). Anything else - KUBE_*, ARGOCD_*,
// KRMGEN_HELM_* and the like - must be passed explicitly by the caller so an
// ambient variable on the host or CI runner can never silently change a
// golden's output. See docs/specification.md for the environment variables
// krmgen itself reads.
func minimalEnv() []string {
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	if tmp, ok := os.LookupEnv("TMPDIR"); ok {
		env = append(env, "TMPDIR="+tmp)
	}
	return env
}

// runScenario copies the fixture to a temp directory, points it at a local
// chart repository, and runs krmgen against it. extraEnv carries any
// scenario-specific variables (e.g. ARGOCD_ENV_MESSAGE) as "KEY=VALUE"
// entries - the child process gets exactly PATH/HOME/TMPDIR, the chart repo
// URL, and these, never the ambient environment.
func runScenario(t *testing.T, name string, extraEnv ...string) result {
	t.Helper()

	fixture := filepath.Join("fixtures", name)
	workDir := filepath.Join(t.TempDir(), name)
	if err := os.CopyFS(workDir, os.DirFS(fixture)); err != nil {
		t.Fatalf("copying fixture %s: %v", name, err)
	}
	// golden.yaml is the expectation, not an input
	_ = os.Remove(filepath.Join(workDir, "golden.yaml"))

	cmd := exec.Command(binaryPath(t), "generate", workDir)
	cmd.Env = append(minimalEnv(), "ARGOCD_ENV_CHART_REPO="+chartRepo(t))
	cmd.Env = append(cmd.Env, extraEnv...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return result{stdout: stdout.String(), stderr: stderr.String(), exitCode: exitCodeOf(t, err)}
}

// runBinary runs krmgen with arbitrary arguments and no fixture, using the
// same minimal, explicit environment as runScenario.
func runBinary(t *testing.T, args ...string) result {
	t.Helper()
	cmd := exec.Command(binaryPath(t), args...)
	cmd.Env = minimalEnv()
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
	checkToolVersions(t)
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

// diff produces a line-based diff of want vs got using an LCS backtrace, so
// a single inserted or deleted line is reported as exactly that - one "+" or
// "-" line - instead of an index-by-index comparison cascading into a bogus
// mismatch on every line that follows it.
func diff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	n, m := len(wantLines), len(gotLines)

	// lcs[i][j] holds the length of the longest common subsequence of
	// wantLines[i:] and gotLines[j:].
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if wantLines[i] == gotLines[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var b strings.Builder
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && wantLines[i] == gotLines[j]:
			i++
			j++
		case j < m && (i == n || lcs[i][j+1] >= lcs[i+1][j]):
			fmt.Fprintf(&b, "+ got  line %d: %q\n", j+1, gotLines[j])
			j++
		default:
			fmt.Fprintf(&b, "- want line %d: %q\n", i+1, wantLines[i])
			i++
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

func TestGolden_KustomizeOnly(t *testing.T) {
	res := runScenario(t, "kustomize-only")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "kustomize-only", res.stdout)
}

func TestGolden_HelmWithKustomize(t *testing.T) {
	res := runScenario(t, "helm-with-kustomize")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "helm-with-kustomize", res.stdout)
	if strings.Contains(res.stdout, "Pulled:") || strings.Contains(res.stdout, "Digest:") {
		t.Error("helm banner leaked into the output")
	}
}

func TestGolden_SkipPatterns(t *testing.T) {
	res := runScenario(t, "skip-patterns")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "skip-patterns", res.stdout)
}

func TestGolden_TemplateFunctions(t *testing.T) {
	// Passed explicitly rather than via t.Setenv: the harness gives the
	// subprocess a minimal, explicit environment (see minimalEnv) and does
	// not inherit the test binary's own env, so t.Setenv here would never
	// reach the child.
	res := runScenario(t, "template-functions", "ARGOCD_ENV_MESSAGE=from-env")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertGolden(t, "template-functions", res.stdout)
	if !strings.Contains(res.stdout, "from-env") {
		t.Error("argocdEnv value did not reach the rendered output")
	}
	if !strings.Contains(res.stdout, "fallback-release") {
		t.Error("kubeEnv default did not reach the release name")
	}
}

// TestGolden_StdoutCarriesOnlyYaml asserts that stdout, in its entirety,
// decodes as a stream of YAML documents. Log lines (from logrus or the
// standard library) go to stderr, never stdout, so a substring check for
// "level=" / "time=" prefixes can never fail - it does not test what its
// name promises. Decoding is the real assertion: it fails on any malformed
// document and on empty output (a decoder that reports zero documents),
// closing the degenerate-output hole the substring check missed.
func TestGolden_StdoutCarriesOnlyYaml(t *testing.T) {
	res := runScenario(t, "helm-with-kustomize")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	assertOnlyYaml(t, res.stdout)
}

// assertOnlyYaml decodes stdout as a stream of YAML documents, requiring
// every document to parse and at least one document to be present.
func assertOnlyYaml(t *testing.T, stdout string) {
	t.Helper()
	dec := yaml.NewDecoder(strings.NewReader(stdout))
	count := 0
	for {
		var doc any
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("stdout document %d does not parse as YAML: %v\nstdout:\n%s", count+1, err, stdout)
		}
		count++
	}
	if count == 0 {
		t.Fatalf("stdout contained no YAML documents\nstdout:\n%s", stdout)
	}
}
