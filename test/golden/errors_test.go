package golden

import (
	"os"
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

// TestError_WorkingDirectoryRemovedOnFailure covers what used to be a
// documented deviation: log.Fatal skipped the deferred cleanup, so a failing
// run left a working directory full of rendered templates behind. The run is
// given a TMPDIR of its own so nothing else on the host can be mistaken for
// krmgen's leftovers.
func TestError_WorkingDirectoryRemovedOnFailure(t *testing.T) {
	tmp := t.TempDir()
	res := runScenario(t, "two-kustomizations", "TMPDIR="+tmp)
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "krmgen") {
			t.Errorf("working directory %q survived a failed run", e.Name())
		}
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
	// The specification promises a failing run prints no usage block. Cobra
	// dumps one on every RunE error unless SilenceUsage is set, so this is
	// the assertion that keeps that setting from being dropped from
	// cmd/root.go by accident. It only guards the root command's setting:
	// cobra suppresses the usage block when either the root's or the
	// executed subcommand's SilenceUsage is true (see (*Command).Execute in
	// spf13/cobra), so removing the (redundant) SilenceUsage on
	// cmd/generate.go's own command would not make this test fail - the
	// root's setting alone still silences it.
	if strings.Contains(res.stderr, "Usage:") || strings.Contains(res.stdout, "Usage:") {
		t.Errorf("want no usage block on a failure\nstderr: %s\nstdout: %s", res.stderr, res.stdout)
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

// TestError_Contract pins what every failing scenario must keep doing across
// the phase-5 refactoring: exit with code 1, name the cause in stderr, and
// (where nothing has been emitted yet) leave stdout empty. The message format
// around the substring is free to change - the substring is not.
func TestError_Contract(t *testing.T) {
	cases := []struct {
		scenario     string
		stableSubstr string
		wantEmptyOut bool
	}{
		{scenario: "two-kustomizations", stableSubstr: "multiple kustomization files", wantEmptyOut: true},
		{scenario: "bad-repo-scheme", stableSubstr: "not supported by any generator", wantEmptyOut: true},
		{scenario: "nested-kustomization", stableSubstr: "unable to find one of 'kustomization.yaml'", wantEmptyOut: true},
		// multi-config-kustomize has already written the first config's block
		// to stdout by the time it fails - see the deviation in the specification.
		{scenario: "multi-config-kustomize", stableSubstr: "already registered id", wantEmptyOut: false},
	}

	for _, tc := range cases {
		t.Run(tc.scenario, func(t *testing.T) {
			res := runScenario(t, tc.scenario)
			if res.exitCode != 1 {
				t.Errorf("exit code = %d, want 1", res.exitCode)
			}
			if !strings.Contains(res.stderr, tc.stableSubstr) {
				t.Errorf("stderr = %q, want it to contain %q", res.stderr, tc.stableSubstr)
			}
			if tc.wantEmptyOut && res.stdout != "" {
				t.Errorf("stdout = %q, want nothing on a failure", res.stdout)
			}
			if !tc.wantEmptyOut && res.stdout == "" {
				t.Error("want the partial output that preceded the failure")
			}
		})
	}
}
