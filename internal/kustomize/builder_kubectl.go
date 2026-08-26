package kustomize

import "fmt"

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
		return "", fmt.Errorf("run kubectl kustomize failed error: %s reason: %s", err, stdErr)
	}
	return stdOut, nil
}
