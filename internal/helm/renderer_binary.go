package helm

import (
	"fmt"
	"strings"

	types "github.com/librucha/krmgen/internal"
)

// binaryRenderer renders by running the helm binary on the host, located via
// helmExecutable() (KRMGEN_HELM_EXECUTABLE, falling back to PATH).
type binaryRenderer struct{}

func newBinaryRenderer() Renderer { return binaryRenderer{} }

func (binaryRenderer) Name() string { return "helm binary" }

func (binaryRenderer) Render(cfg *types.HelmChart, g generator, workDir string) (string, error) {
	args := []string{
		"template",
		cfg.ReleaseName,
		"--include-crds",
	}
	if cfg.Version != "" {
		args = append(args, "--version", cfg.Version)
	}
	if cfg.Namespace != "" {
		args = append(args, "--namespace", cfg.Namespace)
	}

	args = g.addRepoArgs(args)

	if credentialsProvided(cfg) {
		if err := g.login(); err != nil {
			return "", err
		}
		args = g.addCredentials(args)
	}

	valuesArgs, err := getValuesArgs(cfg, workDir)
	if err != nil {
		return "", err
	}
	args = append(args, valuesArgs...)

	executable, err := helmExecutable()
	if err != nil {
		return "", err
	}
	stdOut, stdErr, err := runCommand(executable, args...)
	if err != nil {
		return "", fmt.Errorf("run command %q finished with error %v. Error output %v", executable, err, stdErr)
	}
	return stripHelmBanner(stdOut), nil
}

// helmBannerPrefixes are informational lines Helm v4 writes to stdout (not stderr)
// before the rendered manifests when a chart is pulled from an OCI registry.
// They are not valid YAML and break downstream processing (e.g. kustomize).
//
// This is a permanent property of the binary path only: the library
// implementation talks to helm's Go API directly and never produces this
// banner.
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
