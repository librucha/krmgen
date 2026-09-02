package golden

import (
	"os"
	"os/exec"
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

// secretMarker is the recognisable fragment of the secret value in the
// secret-render-failure fixture. It must never reach stderr.
const secretMarker = "LEAKED-SECRET-MARKER"

// TestError_RenderFailureDoesNotLeakSecretValues covers a reported secret
// disclosure: kustomize reports a resource it cannot serialize by prefixing
// the reason with a %#v dump of the resource's entire map, so a failing
// render used to print every value of a generated Secret to stderr - and,
// under ArgoCD, into the application's sync log. Both backends are checked:
// kubectl embeds the same kustomize and leaked the same dump.
//
// The fixture reaches that failure through a key long enough that kyaml folds
// the line. The reported route - a multi-line value in an env file - no longer
// reaches a backend at all; checkGeneratorEnvFiles rejects it first, which
// TestError_MultilineEnvValueIsRejected covers.
func TestError_RenderFailureDoesNotLeakSecretValues(t *testing.T) {
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		t.Fatalf("kubectl is required to check the external backend: %v", err)
	}

	backends := []struct {
		name string
		env  []string
	}{
		{name: "embedded"},
		{name: "external", env: []string{"KRMGEN_KUBECTL_EXECUTABLE=" + kubectlPath}},
	}

	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			res := runScenario(t, "secret-render-failure", b.env...)
			if res.exitCode != 1 {
				t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
			}
			if strings.Contains(res.stderr, secretMarker) {
				t.Errorf("stderr leaked the secret value:\n%s", res.stderr)
			}
			if strings.Contains(res.stdout, secretMarker) {
				t.Errorf("stdout leaked the secret value:\n%s", res.stdout)
			}
		})
	}
}

// The embedded backend has the rendered resources in hand, so it can say
// which one failed. That is the whole point of redacting rather than
// dropping the error: the message must stay actionable.
func TestError_RenderFailureNamesTheOffendingResource(t *testing.T) {
	res := runScenario(t, "secret-render-failure")
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, "Secret demo") {
		t.Errorf("stderr = %q, want it to name the resource that could not be serialized", res.stderr)
	}
}

// TestError_MultilineEnvValueIsRejected covers the second half of the same
// report. An env file's .properties format cannot carry a newline: kustomize
// truncates the value at the first line break and reads every following line
// as a further key, so a key vault secret ends up both destroyed and written
// out as an unencoded key name. kustomize means to reject those keys but its
// validator is an unimplemented stub, so krmgen is the last place that can.
func TestError_MultilineEnvValueIsRejected(t *testing.T) {
	res := runScenario(t, "multiline-env-value")
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
	}
	if strings.Contains(res.stdout, secretMarker) {
		t.Errorf("stdout contains the secret as a key - the render should not have happened:\n%s", res.stdout)
	}
	// The offending line is a fragment of the value, so the error names the
	// file and the line rather than quoting it.
	if strings.Contains(res.stderr, secretMarker) {
		t.Errorf("stderr leaked the secret value:\n%s", res.stderr)
	}
	for _, want := range []string{"resources/secrets.properties", "line 2", `"files"`} {
		if !strings.Contains(res.stderr, want) {
			t.Errorf("stderr = %q, want it to contain %q", res.stderr, want)
		}
	}
}

// The check runs before a backend is selected, so it must behave the same
// with the external one - otherwise opting into kubectl would opt out of the
// protection.
func TestError_MultilineEnvValueIsRejectedOnBothBackends(t *testing.T) {
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		t.Fatalf("kubectl is required to check the external backend: %v", err)
	}
	res := runScenario(t, "multiline-env-value", "KRMGEN_KUBECTL_EXECUTABLE="+kubectlPath)
	if res.exitCode != 1 {
		t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
	}
	if strings.Contains(res.stderr, secretMarker) || strings.Contains(res.stdout, secretMarker) {
		t.Errorf("the secret value reached the output\nstderr: %s\nstdout: %s", res.stderr, res.stdout)
	}
	if !strings.Contains(res.stderr, "is read as a key") {
		t.Errorf("stderr = %q, want the same rejection the embedded backend gives", res.stderr)
	}
}

// patMarker is the fake credential in the remote-base-credentials fixture.
const patMarker = "LEAKED-PAT-MARKER-12345"

// TestError_RemoteBaseCredentialsAreMasked covers a reported credential
// disclosure. A kustomization may pull a remote base over HTTPS with the
// credential in the URL - resolved from a key vault by a template, so the
// repository itself stores no secret - and when the fetch fails kustomize
// echoes the resolved URL verbatim, twice: naming the resource it could not
// accumulate, and quoting the git command line it ran. Under ArgoCD that
// lands in the application's retained sync log. Both backends are checked;
// they produce the same text.
func TestError_RemoteBaseCredentialsAreMasked(t *testing.T) {
	kubectlPath, err := exec.LookPath("kubectl")
	if err != nil {
		t.Fatalf("kubectl is required to check the external backend: %v", err)
	}

	backends := []struct {
		name string
		env  []string
	}{
		{name: "embedded"},
		{name: "external", env: []string{"KRMGEN_KUBECTL_EXECUTABLE=" + kubectlPath}},
	}

	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			res := runScenario(t, "remote-base-credentials", b.env...)
			if res.exitCode != 1 {
				t.Fatalf("exit code = %d, want 1\nstderr: %s", res.exitCode, res.stderr)
			}
			if strings.Contains(res.stderr, patMarker) {
				t.Errorf("stderr leaked the credential:\n%s", res.stderr)
			}
			if strings.Contains(res.stdout, patMarker) {
				t.Errorf("stdout leaked the credential:\n%s", res.stdout)
			}
			// Masked, not dropped: the repository still has to be
			// identifiable or the error is not worth printing.
			if !strings.Contains(res.stderr, "https://user:***@git.invalid/org/_git/repo") {
				t.Errorf("stderr = %q, want the masked URL to name the repository", res.stderr)
			}
		})
	}
}
