package golden

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// The golden files under fixtures/*/golden.yaml are byte-for-byte captures
// of krmgen's output, which in turn depends on the exact helm and kubectl
// (Kustomize) versions used to render it — see docs/specification.md,
// "Known differences between helm versions". Regenerating goldens for every
// supported tool version is not the goal: krmgen's support matrix (helm
// 3.8.0+, including 4.x) stays as documented, but the goldens themselves are
// anchored to one reference pair, chosen as whatever was installed on the
// machine that generated them.
//
// anchorHelmVersion is the `helm version --short` output of that reference
// helm (a released v4.2.4 build; the "+g..." suffix is helm's normal
// embedded-commit build metadata, not a dev build).
const anchorHelmVersion = "v4.2.4+g3900f43"

// anchorKustomizeVersion is the "Kustomize Version" line reported by
// `kubectl version --client` for the reference kubectl (v1.36.3), which is
// what actually determines kustomize's rendering behaviour — see
// docs/specification.md, "Kustomize version follows kubectl".
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
