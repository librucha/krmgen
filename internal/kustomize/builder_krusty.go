package kustomize

import (
	"fmt"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// krustyBuilder renders with the kustomize library compiled into krmgen, so
// the version is pinned in go.mod rather than being whatever kubectl the host
// happens to have.
type krustyBuilder struct{}

func newKrustyBuilder() Builder { return krustyBuilder{} }

func (krustyBuilder) Name() string { return "embedded kustomize" }

func (krustyBuilder) Build(dir string) (string, error) {
	opts := krusty.MakeDefaultOptions()
	// kubectl kustomize applies the legacy resource ordering; the library
	// defaults to none. Without this every rendered document would be
	// reordered relative to what krmgen has always produced.
	opts.Reorder = krusty.ReorderOptionLegacy

	result, err := krusty.MakeKustomizer(opts).Run(filesys.MakeFsOnDisk(), dir)
	if err != nil {
		return "", fmt.Errorf("run kustomize failed: %w", err)
	}
	out, err := result.AsYaml()
	if err != nil {
		return "", fmt.Errorf("rendering kustomize output failed: %w", err)
	}
	return string(out), nil
}
