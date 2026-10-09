package config

import (
	"github.com/librucha/krmgen/v2/internal"
	"github.com/librucha/krmgen/v2/internal/helm"
	"github.com/librucha/krmgen/v2/internal/kustomize"
	"strings"
)

func ProcessConfig(config *types.Config, workDir string) (string, error) {
	resources := strings.Builder{}
	if config.HasHelm() {
		helmCharts, err := helm.TemplateHelmCharts(config.Helm, workDir)
		if err != nil {
			return "", err
		}
		resources.WriteString(helmCharts)
	}
	kustomizeFile, err := kustomize.FindKustomizeFile(workDir)
	if err != nil {
		return "", err
	}
	if kustomizeFile != "" {
		return kustomize.BuildKustomize(kustomizeFile, workDir, resources.String())
	}

	return resources.String(), nil
}
