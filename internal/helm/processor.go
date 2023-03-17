package helm

import (
	"fmt"
	"github.com/google/uuid"
	types "github.com/librucha/krmgen/internal"
	"github.com/librucha/krmgen/internal/tool"
	cons "github.com/librucha/krmgen/internal/utils"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func helmExecutable() string {
	helm, found := os.LookupEnv(cons.EnvHelmExecutable)
	if !found {
		path, err := exec.LookPath("helm")
		if err != nil {
			log.Fatalf("helm executable not found in OS")
		}
		return path
	}
	return helm
}

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
	config := generator.getConfig()
	tempDir, err := os.MkdirTemp(os.TempDir(), config.ReleaseName)
	if err != nil {
		return "", err
	}
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tempDir)

	args := []string{
		"template",
		config.ReleaseName,
		"--include-crds",
	}
	if config.Version != "" {
		args = append(args, "--version", config.Version)
	}

	args = generator.addRepoArgs(args)

	if credentialsProvided(generator.getConfig()) {
		generator.login()
		args = generator.addCredentials(args)
	}

	valuesArgs, err := getValuesArgs(config, workDir, tempDir)
	if err != nil {
		return "", err
	}
	args = append(args, valuesArgs...)

	stdOut, stdErr, err := tool.RunCommand(helmExecutable(), args...)
	if err != nil {
		return "", fmt.Errorf("run command %q finished with error %v. Error output %v", helmExecutable(), err, stdErr)
	}
	return stdOut, nil
}

func getValuesArgs(helmChartConfig *types.HelmChart, workDir string, tempDir string) ([]string, error) {
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
		valuesInlineFile := filepath.Join(tempDir, "helm-values-"+helmChartConfig.ReleaseName+uuid.NewString())
		err = os.WriteFile(valuesInlineFile, valuesInlineYaml, 0666)
		if err != nil {
			return nil, err
		}
		args = append(args, "--values", valuesInlineFile)
	}
	return args, nil
}
