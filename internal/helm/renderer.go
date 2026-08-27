package helm

import (
	"os"

	types "github.com/librucha/krmgen/internal"
	cons "github.com/librucha/krmgen/internal/utils"
)

// Renderer turns one chart declaration into rendered YAML. Two implementations
// exist for the whole lifetime of the project: the helm binary on the host, and
// the helm library compiled into krmgen. See docs/specification.md, section 5.
type Renderer interface {
	// Render returns the manifests for one chart, with no trailing newline
	// normalisation - the caller concatenates results verbatim.
	Render(cfg *types.HelmChart, g generator, workDir string) (string, error)
	// Name identifies the backend in errors and tests.
	Name() string
}

// selectRenderer decides which backend renders. The embedded helm library is
// the default; setting KRMGEN_HELM_EXECUTABLE opts into the external helm
// binary. An empty value is treated as unset, consistent with helmExecutable
// itself (internal/helm/processor.go) and with selectBuilder
// (internal/kustomize/builder.go).
func selectRenderer() Renderer {
	if executable, found := os.LookupEnv(cons.EnvHelmExecutable); found && executable != "" {
		return newBinaryRenderer()
	}
	return newSDKRenderer()
}
