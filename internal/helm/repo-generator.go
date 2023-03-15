package helm

import (
	"fmt"
	types "github.com/librucha/krmgen/internal"
	"github.com/librucha/krmgen/internal/tool"
	"log"
	"regexp"
	"strings"
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

func newRepoHelmGenerator(config *types.HelmChart) repoHelmGenerator {
	g := repoHelmGenerator{config}
	g.addRepoIfNecessary()
	return g
}

func (g repoHelmGenerator) addRepoIfNecessary() {
	helm := helmExecutable()

	// if no repo added this command returns error :)
	out, _, _ := tool.RunCommand(helm, "repo", "list")
	repos := strings.Split(out, "\n")
	for _, repoLine := range repos {
		if strings.Contains(repoLine, g.config.RepoUrl) && strings.Contains(repoLine, g.chartIdShort()) {
			return
		}
	}

	_, _, err := tool.RunCommand(helm, "repo", "add", g.chartIdShort(), g.config.RepoUrl)
	if err != nil {
		log.Fatalf("add helm repo %q url %q failed reason: %q", g.chartIdShort(), g.config.RepoUrl, err.Error())
	}
	g.updateRepo()
}

func (g repoHelmGenerator) updateRepo() {
	_, _, err := tool.RunCommand(helmExecutable(), "repo", "update")
	if err != nil {
		log.Fatalf("update helm repos failed reason: %q", err.Error())
	}
}
