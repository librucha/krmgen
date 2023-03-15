package helm

import (
	"fmt"
	types "github.com/librucha/krmgen/internal"
	"regexp"
)

var helmUrlRegexp = regexp.MustCompile("\\w+://([0-9a-zA-Z-_]+).*")

type repoHelmGenerator struct {
	config *types.HelmChart
}

func (g repoHelmGenerator) getConfig() *types.HelmChart {
	return g.config
}

func (g repoHelmGenerator) chartId() string {
	return fmt.Sprintf("%s/%s", g.chartIdShort(), g.config.Name)
}

func (g repoHelmGenerator) chartIdShort() string {
	res := helmUrlRegexp.FindStringSubmatch(g.config.RepoUrl)
	if len(res) > 1 {
		return res[1]
	}
	return g.config.RepoUrl
}

func (g repoHelmGenerator) login() {
	// login on helm repo is not supported
}

func (g repoHelmGenerator) addCredentials(in []string) []string {
	return append(in, credentialsArgs(g.config)...)
}

func (g repoHelmGenerator) addRepoArgs(in []string) []string {
	return append(in, "--repo", g.config.RepoUrl, "--release-name", g.config.Name)
}

func newRepoHelmGenerator(config *types.HelmChart) repoHelmGenerator {
	g := repoHelmGenerator{config}
	return g
}
