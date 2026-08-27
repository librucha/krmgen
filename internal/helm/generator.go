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
	login() error
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
	u, p := credentials(config)
	return u != "" || p != ""
}

// credentials resolves the effective username and password for a chart: the
// config value, falling back to KRMGEN_HELM_USERNAME/KRMGEN_HELM_PASSWORD,
// or both empty when IgnoreCredentials is set. This is the single place
// that reads credentials - every renderer formats these values its own way
// instead of duplicating the lookup.
func credentials(config *types.HelmChart) (username, password string) {
	if config.IgnoreCredentials {
		return "", ""
	}
	username = config.Username
	if username == "" {
		username = os.Getenv(cons.EnvHelmUsername)
	}
	password = config.Password
	if password == "" {
		password = os.Getenv(cons.EnvHelmPassword)
	}
	return username, password
}

// username is the credentials helper the SDK renderer uses to fill
// action.Install.ChartPathOptions.Username.
func username(config *types.HelmChart) string {
	u, _ := credentials(config)
	return u
}

// password is the credentials helper the SDK renderer uses to fill
// action.Install.ChartPathOptions.Password.
func password(config *types.HelmChart) string {
	_, p := credentials(config)
	return p
}

// credentialsArgs formats credentials as `helm` CLI flags, for renderers and
// generators that shell out to the binary.
func credentialsArgs(config *types.HelmChart) []string {
	var args []string
	username, password := credentials(config)
	if username != "" {
		args = append(args, "--username", username)
	}
	if password != "" {
		args = append(args, "--password", password)
	}
	return args
}
