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

	args := []string{
		"template",
		config.ReleaseName,
		"--include-crds",
	}
	if config.Version != "" {
		args = append(args, "--version", config.Version)
	}
	if config.Namespace != "" {
		args = append(args, "--namespace", config.Namespace)
	}

	args = generator.addRepoArgs(args)

	if credentialsProvided(generator.getConfig()) {
		generator.login()
		args = generator.addCredentials(args)
	}

	valuesArgs, err := getValuesArgs(config, workDir)
	if err != nil {
		return "", err
	}
	args = append(args, valuesArgs...)

	stdOut, stdErr, err := tool.RunCommand(helmExecutable(), args...)
	if err != nil {
		return "", fmt.Errorf("run command %q finished with error %v. Error output %v", helmExecutable(), err, stdErr)
	}
	return stripHelmBanner(stdOut), nil
}

// helmBannerPrefixes are informational lines Helm v4 writes to stdout (not stderr)
// before the rendered manifests when a chart is pulled from an OCI registry.
// They are not valid YAML and break downstream processing (e.g. kustomize).
var helmBannerPrefixes = []string{
	"Pulled: ",
	"Digest: ",
	"Signed by: ",
	"Chart Hash Verified: ",
}

// stripHelmBanner removes the Helm banner lines from the beginning of helm template output.
func stripHelmBanner(output string) string {
	for {
		line, rest, _ := strings.Cut(output, "\n")
		if !isHelmBannerLine(line) {
			return output
		}
		output = rest
	}
}

func isHelmBannerLine(line string) bool {
	line = strings.TrimSuffix(line, "\r")
	for _, prefix := range helmBannerPrefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
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
		err = os.WriteFile(valuesInlineFile, valuesInlineYaml, 0666)
		if err != nil {
			return nil, err
		}
		args = append(args, "--values", valuesInlineFile)
	}
	return args, nil
}
