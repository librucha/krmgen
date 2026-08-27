package helm

import (
	"fmt"
	types "github.com/librucha/krmgen/internal"
	"regexp"
	"strings"
)

var helmRegistryRegexp = regexp.MustCompile(`\w+://([0-9a-zA-Z-_.]+)/.*`)

type ociHelmGenerator struct {
	config *types.HelmChart
}

func (g ociHelmGenerator) getConfig() *types.HelmChart {
	return g.config
}

func (g ociHelmGenerator) chartId() string {
	var normalizedRepo = g.config.RepoUrl
	if !strings.HasSuffix(normalizedRepo, "/") {
		normalizedRepo += "/"
	}
	return normalizedRepo + g.config.Name
}

func (g ociHelmGenerator) chartIdShort() string {
	res := helmRegistryRegexp.FindStringSubmatch(g.chartId())
	if len(res) > 1 {
		return res[1]
	}
	return g.chartId()
}

func (g ociHelmGenerator) login() error {
	args := []string{"registry", "login", g.chartIdShort()}
	args = g.addCredentials(args)
	executable, err := helmExecutable()
	if err != nil {
		return err
	}
	if _, _, err := runCommand(executable, args...); err != nil {
		return fmt.Errorf("login to helm registry %q failed reason: %q", g.chartIdShort(), err.Error())
	}
	return nil
}

func (g ociHelmGenerator) addCredentials(in []string) []string {
	return append(in, credentialsArgs(g.config)...)
}

func (g ociHelmGenerator) addRepoArgs(in []string) []string {
	return append(in, g.config.RepoUrl)
}

func newOciHelmGenerator(config *types.HelmChart) ociHelmGenerator {
	return ociHelmGenerator{config}
}
