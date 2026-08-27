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

// forceRenderer overrides selectRenderer when non-nil. It is never set in a
// default build - the only assignment lives behind the forcesdk build tag
// (renderer_forcesdk.go), a seam for the golden differential test that lets
// it drive the SDK renderer unconditionally before selectRenderer itself
// gains real branching (see docs/specification.md, section 5, and task 4 of
// the helm-sdk phase).
var forceRenderer Renderer

func templateHelm(generator generator, workDir string) (string, error) {
	renderer := forceRenderer
	if renderer == nil {
		renderer = selectRenderer()
	}
	return renderer.Render(generator.getConfig(), generator, workDir)
}

// valueFiles resolves a chart's values into a list of file paths - the
// declared values file, joined with workDir, followed by valuesInline
// written out to a temp file. This is the single place that reads values;
// each renderer formats the result its own way (the binary renderer turns
// it into repeated --values flags, the SDK renderer feeds it to
// values.Options.ValueFiles directly).
func valueFiles(helmChartConfig *types.HelmChart, workDir string) ([]string, error) {
	var files []string
	valuesFile := helmChartConfig.ValuesFile
	if valuesFile != "" {
		files = append(files, filepath.Join(workDir, valuesFile))
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
		files = append(files, valuesInlineFile)
	}
	return files, nil
}

func getValuesArgs(helmChartConfig *types.HelmChart, workDir string) ([]string, error) {
	files, err := valueFiles(helmChartConfig, workDir)
	if err != nil {
		return nil, err
	}
	var args []string
	for _, f := range files {
		args = append(args, "--values", f)
	}
	return args, nil
}
