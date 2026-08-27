package helm

import (
	types "github.com/librucha/krmgen/internal"
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

// selectRenderer decides which backend renders. For now it always returns the
// binary renderer; a second implementation and the selection logic between
// them are introduced in a later phase.
func selectRenderer() Renderer {
	return newBinaryRenderer()
}
