package golden

import (
	"debug/buildinfo"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// The golden files under fixtures/*/golden.yaml are byte-for-byte captures
// of krmgen's output, which in turn depends on the exact helm version used
// to render it — see docs/specification.md, "Known differences between helm
// versions". On the default kustomize path, rendering behaviour is pinned to
// the sigs.k8s.io/kustomize/api version compiled into krmgen (see
// anchorKustomizeAPIVersion and TestEmbeddedKustomizeMatchesTheAnchor,
// below), not to whatever kubectl happens to embed. The installed kubectl's
// Kustomize version still has to match a reference too, because the same
// goldens are also asserted against the external backend
// (KRMGEN_KUBECTL_EXECUTABLE) by TestGolden_ExternalBackendMatchesTheGoldens
// and compared against the embedded backend by TestGolden_BothBackendsAgree
// and TestGolden_BothBackendsAgreeOnErrors — see docs/specification.md,
// "Kustomize version follows kubectl — external backend only". Regenerating
// goldens for every supported tool version is not the goal: krmgen's support
// matrix (helm 3.8.0+, including 4.x) stays as documented, but the goldens
// themselves are anchored to one reference pair, chosen as whatever was
// installed on the machine that generated them.
//
// anchorHelmVersion is the `helm version --short` output of that reference
// helm (a released v4.2.4 build; the "+g..." suffix is helm's normal
// embedded-commit build metadata, not a dev build).
const anchorHelmVersion = "v4.2.4+g3900f43"

// anchorKustomizeVersion is the "Kustomize Version" line reported by
// `kubectl version --client` for the reference kubectl (v1.36.3). It only
// determines rendering behaviour on the external backend
// (KRMGEN_KUBECTL_EXECUTABLE); the default path is pinned to
// anchorKustomizeAPIVersion instead — see docs/specification.md, "Kustomize
// version follows kubectl — external backend only".
const anchorKustomizeVersion = "v5.8.1"

var (
	versionCheckOnce sync.Once
	versionCheckErr  error
)

// checkToolVersions fails the test loudly, naming the expected and actual
// versions, if the running helm or kubectl does not match the exact pair the
// goldens were generated against. Without this, a version drift shows up as
// an opaque golden diff that reads like a product regression.
func checkToolVersions(t *testing.T) {
	t.Helper()
	versionCheckOnce.Do(func() {
		versionCheckErr = doCheckToolVersions()
	})
	if versionCheckErr != nil {
		t.Fatalf("%v", versionCheckErr)
	}
}

func doCheckToolVersions() error {
	gotHelm, err := runningHelmVersion()
	if err != nil {
		return err
	}
	if gotHelm != anchorHelmVersion {
		return fmt.Errorf(
			"helm version mismatch: goldens under test/golden/fixtures were generated with helm %s, but %s is installed. "+
				"This is a tooling drift, not a product regression - install the anchor version, or update anchorHelmVersion "+
				"in test/golden/versions_test.go and regenerate the goldens with -update after reviewing every diff",
			anchorHelmVersion, gotHelm)
	}

	gotKustomize, err := runningKustomizeVersion()
	if err != nil {
		return err
	}
	if gotKustomize != anchorKustomizeVersion {
		return fmt.Errorf(
			"kustomize version mismatch: goldens under test/golden/fixtures were generated with kubectl embedding kustomize %s, "+
				"but %s is installed. This is a tooling drift, not a product regression - install a kubectl embedding the anchor "+
				"kustomize version, or update anchorKustomizeVersion in test/golden/versions_test.go and regenerate the goldens "+
				"with -update after reviewing every diff",
			anchorKustomizeVersion, gotKustomize)
	}
	return nil
}

func runningHelmVersion() (string, error) {
	out, err := exec.Command("helm", "version", "--short").Output()
	if err != nil {
		return "", fmt.Errorf("running helm version --short: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// anchorKustomizeAPIVersion is the sigs.k8s.io/kustomize/api version compiled
// into krmgen. Output was verified byte-identical between this version and
// the kubectl anchored above; a different pair may render differently, and
// then a golden diff is a tooling change, not a product regression.
const anchorKustomizeAPIVersion = "v0.21.1"

// TestEmbeddedKustomizeMatchesTheAnchor reads the module dependency versions
// embedded in the built krmgen binary (the same one the golden scenarios
// exec) via debug/buildinfo.ReadFile.
//
// The obvious alternative - runtime/debug.ReadBuildInfo() called from inside
// this test itself, on the theory that the test binary and krmgen compile
// from the same go.mod so either reports the same versions - does not work:
// `go test` only populates BuildInfo.Deps when the package under test is
// itself `package main`. For any library package, including this one, Deps
// comes back empty regardless of what is actually linked in, so every
// dependency lookup would report "not among the build dependencies" even
// when the compiled-in version matches the anchor exactly (verified against
// go 1.21.5, 1.26.0 and 1.26.3). Reading the built krmgen binary from disk
// sidesteps the limitation entirely and inspects the actual shipped
// artifact rather than a proxy for it.
func TestEmbeddedKustomizeMatchesTheAnchor(t *testing.T) {
	bin := binaryPath(t)
	info, err := buildinfo.ReadFile(bin)
	if err != nil {
		t.Fatalf("reading build info from %s: %v", bin, err)
	}
	for _, dep := range info.Deps {
		if dep.Path == "sigs.k8s.io/kustomize/api" {
			if dep.Version != anchorKustomizeAPIVersion {
				t.Errorf("kustomize/api is %s, goldens were generated against %s - "+
					"this is a tooling change, not a product regression; verify the "+
					"output before updating the anchor", dep.Version, anchorKustomizeAPIVersion)
			}
			return
		}
	}
	t.Error("sigs.k8s.io/kustomize/api is not among the build dependencies")
}

func runningKustomizeVersion() (string, error) {
	out, err := exec.Command("kubectl", "version", "--client").Output()
	if err != nil {
		return "", fmt.Errorf("running kubectl version --client: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "Kustomize Version: "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("kubectl version --client output did not contain a Kustomize Version line:\n%s", out)
}
