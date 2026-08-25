package golden

import (
	"strings"
	"testing"
)

func TestError_TwoKustomizations(t *testing.T) {
	res := runScenario(t, "two-kustomizations")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, "multiple kustomization files") {
		t.Errorf("stderr = %q, want it to name the duplicate kustomization files", res.stderr)
	}
	if res.stdout != "" {
		t.Errorf("stdout = %q, want nothing on a failure", res.stdout)
	}
}

func TestError_UnsupportedRepoScheme(t *testing.T) {
	res := runScenario(t, "bad-repo-scheme")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, "not supported by any generator") {
		t.Errorf("stderr = %q, want it to name the unsupported repository", res.stderr)
	}
}

func TestError_MissingPathArgument(t *testing.T) {
	res := runBinary(t, "generate")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
}
