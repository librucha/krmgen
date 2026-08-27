package helm

import (
	"fmt"
	"github.com/google/uuid"
	types "github.com/librucha/krmgen/internal"
	"github.com/librucha/krmgen/internal/tool"
	cons "github.com/librucha/krmgen/internal/utils"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func helmExecutable() (string, error) {
	if executable, found := os.LookupEnv(cons.EnvHelmExecutable); found && executable != "" {
		return executable, nil
	}
	path, err := exec.LookPath("helm")
	if err != nil {
		return "", fmt.Errorf("helm executable not found in OS")
	}
	return path, nil
}

// runCommand is a seam: tests replace it to observe the helm invocation
// without running the binary.
var runCommand = tool.RunCommand

func TemplateHelmCharts(helmConfig *types.Helm, workDir string) (string, error) {

	helmOutput := strings.Builder{}
	for _, helmChartConfig := range *helmConfig.Charts {
		generator, err := newGenerator(&helmChartConfig)
		if err != nil {
			return "", err
		}

		helmTemplate, err := templateHelm(generator, workDir)
		if err != nil {
			return "", err
		}
		_, err = helmOutput.WriteString(helmTemplate)
		if err != nil {
			return "", err
		}
	}
	return helmOutput.String(), nil
}

func templateHelm(generator generator, workDir string) (string, error) {
	return selectRenderer().Render(generator.getConfig(), generator, workDir)
}

func getValuesArgs(helmChartConfig *types.HelmChart, workDir string) ([]string, error) {
	var args []string
	valuesFile := helmChartConfig.ValuesFile
	if valuesFile != "" {
		filePath := filepath.Join(workDir, valuesFile)
		args = append(args, "--values", filePath)
	}
	if len(helmChartConfig.ValuesInline) > 0 {
		valuesInlineYaml, err := yaml.Marshal(helmChartConfig.ValuesInline)
		if err != nil {
			return nil, err
		}
		valuesInlineFile := filepath.Join(workDir, "helm-values-"+helmChartConfig.ReleaseName+"-"+uuid.NewString())
		err = os.WriteFile(valuesInlineFile, valuesInlineYaml, cons.FilePerm)
		if err != nil {
			return nil, err
		}
		args = append(args, "--values", valuesInlineFile)
	}
	return args, nil
}
