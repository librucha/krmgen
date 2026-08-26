package kustomize

import (
	"os"

	cons "github.com/librucha/krmgen/internal/utils"
)

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

// selectBuilder decides which backend renders. This is the only place that
// reads KRMGEN_KUBECTL_EXECUTABLE: setting it opts into the external tool on
// the host, which is a supported choice for anyone who needs to pin the
// exact kustomize their environment ships. Leaving it unset uses the version
// compiled into krmgen.
//
// Until this phase the variable was declared and never read - the binary was
// always taken from PATH.
func selectBuilder() Builder {
	if executable, found := os.LookupEnv(cons.EnvKubectlExecutable); found && executable != "" {
		return newKubectlBuilder(executable)
	}
	return newKrustyBuilder()
}
