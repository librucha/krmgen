package kustomize

import (
	"fmt"
	"strings"

	cons "github.com/librucha/krmgen/internal/utils"
)

// kubectlBuilder renders by running the kubectl binary, which embeds its own
// copy of kustomize. Which version that is depends on the installed kubectl -
// see docs/specification.md, "Kustomize version follows kubectl".
type kubectlBuilder struct {
	executable string
}

func newKubectlBuilder(executable string) Builder {
	return kubectlBuilder{executable: executable}
}

func (b kubectlBuilder) Name() string { return "kubectl " + b.executable }

func (b kubectlBuilder) Build(dir string) (string, error) {
	stdOut, stdErr, err := runCommand(b.executable, "kustomize", dir)
	if err != nil {
		return "", fmt.Errorf("run kubectl kustomize failed error: %s reason: %s", err, redactRenderDump(stdErr))
	}
	return stdOut, nil
}

// renderDumpMarker is how the kustomize inside kubectl opens the %#v dump of
// a resource it failed to serialize (ResMap.AsYaml). The dump carries every
// value of that resource, so on a generated Secret it is the whole secret in
// clear text.
const renderDumpMarker = "map[string]interface {}{"

// redactRenderDump keeps kubectl's stderr out of the error whenever it
// carries that dump. Across the subprocess boundary there is no resource list
// to inspect, so unlike the embedded backend this cannot name the offending
// resource - the message points at the backend that can instead.
func redactRenderDump(stdErr string) string {
	if !strings.Contains(stdErr, renderDumpMarker) {
		return stdErr
	}
	return "<redacted: kubectl printed the rendered resources, which may contain secrets. " +
		"Unset " + cons.EnvKubectlExecutable + " to render with the embedded kustomize, " +
		"which reports the offending resource without its values>"
}
