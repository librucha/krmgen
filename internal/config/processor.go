package config

import (
	"github.com/librucha/krmgen/internal"
	"github.com/librucha/krmgen/internal/helm"
	"github.com/librucha/krmgen/internal/kustomize"
	"github.com/librucha/krmgen/internal/template"
	"log"
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
	kustomizeFile := kustomize.FindKustomizeFile(workDir)
	if kustomizeFile != "" {
		kustomizeResources := kustomize.BuildKustomize(kustomizeFile, workDir, resources.String())
		resources.Reset()
		resources.WriteString(kustomizeResources)
	}
	// evaluate templates
	evaluated, err := template.EvalGoTemplates(resources.String())
	if err != nil {
		log.Fatalf("template evaluation of result failed error: %s", err)
	}
	return evaluated, nil
}
