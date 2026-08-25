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
