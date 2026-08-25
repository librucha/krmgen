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

// TestError_MultiConfigWithKustomization pins the deviation the specification
// records: krmgen rewrites the kustomization on disk once per config file, so
// the second pass re-accumulates the first pass's generated resource file and
// kustomize rejects the duplicate. Partial output has already reached stdout
// by then.
func TestError_MultiConfigWithKustomization(t *testing.T) {
	res := runScenario(t, "multi-config-kustomize")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, "already registered id") {
		t.Errorf("stderr = %q, want it to name the duplicate resource id", res.stderr)
	}
	if res.stdout == "" {
		t.Error("expected the first config's block to have reached stdout before the failure")
	}
}

// TestError_KustomizationOnlyInSubdirectory pins a second deviation: a
// kustomization nested below the source root is discovered by the recursive
// search but then built from the root, which has none.
func TestError_KustomizationOnlyInSubdirectory(t *testing.T) {
	res := runScenario(t, "nested-kustomization")
	if res.exitCode != 1 {
		t.Errorf("exit code = %d, want 1", res.exitCode)
	}
	if !strings.Contains(res.stderr, "unable to find one of 'kustomization.yaml'") {
		t.Errorf("stderr = %q, want kustomize to report the missing root kustomization", res.stderr)
	}
}
