package helm

import (
	"fmt"
	types "github.com/librucha/krmgen/internal"
	cons "github.com/librucha/krmgen/internal/utils"
	"os"
	"strings"
)

type idProvider interface {
	// id returns chart identification for operations
	// can be oci url or repo/chart combination
	chartId() string
	// chartIdShort returns short version of chartId
	chartIdShort() string
}

type authenticator interface {
	// authenticate to specific helm remote
	login()
	addCredentials([]string) []string
}

type configProvider interface {
	getConfig() *types.HelmChart
}

type generator interface {
	idProvider
	authenticator
	configProvider
	addRepoArgs([]string) []string
}

func newGenerator(config *types.HelmChart) (generator, error) {
	normUrl := strings.ToLower(config.RepoUrl)
	if strings.HasPrefix(normUrl, "oci") {
		return newOciHelmGenerator(config), nil
	}
	if strings.HasPrefix(normUrl, "http") {
		return newRepoHelmGenerator(config), nil
	}
	return nil, fmt.Errorf("helm repo %q is not supported by any generator", config.RepoUrl)
}

// credentialsProvided returns true if username and password are provided some way
func credentialsProvided(config *types.HelmChart) bool {
	return len(credentialsArgs(config)) > 0
}

func credentialsArgs(config *types.HelmChart) []string {
	var args []string
	if config.IgnoreCredentials {
		return args
	}
	username := config.Username
	if username == "" {
		username = os.Getenv(cons.EnvHelmUsername)
	}
	if username != "" {
		args = append(args, "--username", username)
	}
	password := config.Password
	if password == "" {
		password = os.Getenv(cons.EnvHelmPassword)
	}
	if password != "" {
		args = append(args, "--password", password)
	}
	return args
}
