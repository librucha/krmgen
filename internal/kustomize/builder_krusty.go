package kustomize

import (
	"errors"
	"fmt"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
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
		// Deliberately not wrapping err: ResMap.AsYaml prefixes the real
		// reason with a %#v dump of the failing resource's entire map, so
		// %w would print every value of a generated Secret to stderr - the
		// same rendered data internal/utils/perm.go keeps at 0600, and the
		// text ArgoCD retains in an application's sync log.
		return "", fmt.Errorf("rendering kustomize output failed: %w", serializationError(result))
	}
	return string(out), nil
}

// serializationError reconstructs the failure ResMap.AsYaml reported without
// its dump of the rendered resources. It re-runs the same per-resource
// serialization to find the offending resource, and reports only that
// resource's identity plus the underlying reason - a YAML/JSON parser
// message, which describes the syntax it choked on rather than quoting the
// document.
func serializationError(m resmap.ResMap) error {
	for _, res := range m.Resources() {
		if _, err := res.AsYAML(); err != nil {
			return fmt.Errorf("resource %s could not be serialized to YAML: %w", resourceRef(res), err)
		}
	}
	// AsYaml can also fail writing to its own buffer, which no per-resource
	// retry reproduces. There is no resource to name, and the original text
	// is not safe to print, so report the failure without either.
	return errors.New("a rendered resource could not be serialized to YAML")
}

// resourceRef names a resource the way an operator would look it up. Group,
// kind, namespace and name are metadata, never rendered secret material.
func resourceRef(res *resource.Resource) string {
	gvk := res.GetGvk()
	kind := gvk.Kind
	if gvk.Group != "" {
		kind += "." + gvk.Group
	}
	if ns := res.GetNamespace(); ns != "" {
		return fmt.Sprintf("%s %s/%s", kind, ns, res.GetName())
	}
	return fmt.Sprintf("%s %s", kind, res.GetName())
}
