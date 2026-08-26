package kustomize

// Builder renders a prepared kustomization directory into YAML.
//
// Two implementations exist: one shells out to kubectl, one uses the
// kustomize library directly. They are expected to produce identical output;
// where they cannot, the difference is recorded in docs/specification.md
// rather than treated as a defect.
type Builder interface {
	// Build renders the kustomization rooted at dir.
	Build(dir string) (string, error)
	// Name identifies the backend in diagnostics.
	Name() string
}
