package helm

import (
	types "github.com/librucha/krmgen/internal"
	"github.com/librucha/krmgen/internal/tool"
	"log"
	"regexp"
	"strings"
)

var helmRegistryRegexp = regexp.MustCompile("\\w+://([0-9a-zA-Z-_.]+)/.*")

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

func (g ociHelmGenerator) login() {
	args := []string{"registry", "login", g.chartIdShort()}
	args = g.addCredentials(args)

	_, _, err := tool.RunCommand(helmExecutable(), args...)
	if err != nil {
		log.Fatalf("login to helm registry %q failed reason: %q", g.chartIdShort(), err.Error())
	}
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
