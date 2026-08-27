//go:build oci

// These tests reach a public OCI registry. They are excluded from the default
// build because the rest of the suite is hermetic. Run them with:
//
//	go test -tags oci ./test/golden/
//
// A hermetic OCI registry is not possible today: helm requires --plain-http
// for an HTTP registry and --insecure-skip-tls-verify for a self-signed one,
// and krmgen passes neither.
package golden

import (
	"os/exec"
	"strings"
	"testing"
)

func TestOci_BannerIsStripped(t *testing.T) {
	res := runScenario(t, "oci-public")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0\nstderr: %s", res.exitCode, res.stderr)
	}
	if strings.HasPrefix(res.stdout, "Pulled:") || strings.HasPrefix(res.stdout, "Digest:") {
		t.Error("helm's OCI banner reached stdout")
	}
	if !strings.Contains(res.stdout, "kind:") {
		t.Errorf("stdout does not look like rendered YAML: %q", res.stdout[:min(200, len(res.stdout))])
	}
}

// TestOci_BothHelmRenderersAgree is the OCI sibling of
// TestGolden_BothHelmRenderersAgree (harness_test.go): same measurement -
// the embedded helm library and the external helm binary must produce
// byte-identical stdout and agree on exit code for the same scenario - but
// against a real registry (ghcr.io/stakater/charts/reloader), because
// oci-public is the one golden scenario that needs the network and is
// therefore excluded from the default, hermetic suite (see the package
// comment above). This is task 5 of phase 6's measurement: it proves the
// fix in internal/helm/renderer_sdk.go and internal/helm/oci-generator.go
// (registry client built and set on the embedded path, RepoURL left empty
// for oci:// charts) actually renders the same thing as the external
// binary, not just that it renders without error.
func TestOci_BothHelmRenderersAgree(t *testing.T) {
	helmPath, err := exec.LookPath("helm")
	if err != nil {
		t.Fatalf("helm is required to compare renderers: %v", err)
	}

	viaLibrary := runScenario(t, "oci-public")
	if viaLibrary.exitCode != 0 {
		t.Fatalf("embedded (library) renderer exit code = %d, want 0\nstderr: %s", viaLibrary.exitCode, viaLibrary.stderr)
	}
	viaBinary := runScenario(t, "oci-public", "KRMGEN_HELM_EXECUTABLE="+helmPath)
	if viaBinary.exitCode != 0 {
		t.Fatalf("external (binary) renderer exit code = %d, want 0\nstderr: %s", viaBinary.exitCode, viaBinary.stderr)
	}

	if viaBinary.stdout != viaLibrary.stdout {
		t.Errorf("the two helm renderers disagree on the OCI scenario:\n%s", diff(viaBinary.stdout, viaLibrary.stdout))
	}
}
