package helm

import (
	"fmt"
	types "github.com/librucha/krmgen/internal"
	"regexp"
	"strings"

	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/registry"
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

// registryClient builds the OCI registry client sdkRenderer needs to
// authenticate a chart pull on the embedded path.
//
// This is a different authentication mechanism than login, not just a
// different call: login shells out to `helm registry login`, which writes
// credentials into helm's on-disk config file (settings.RegistryConfig) so
// every subsequent helm invocation on the host picks them up. Here, when
// credentials are available, they are handed to the registry client
// in-memory via registry.ClientOptBasicAuth and never touch disk or outlive
// this *registry.Client. When no credentials are available (e.g.
// ignoreCredentials, or a public registry such as the oci-public golden
// scenario), the client falls back to whatever registry.NewClient finds in
// settings.RegistryConfig - the same file the external binary's `helm
// registry login` would have written to, read here but never written to -
// mirroring newDefaultRegistryClient in helm's own
// pkg/cmd/template.go (helm.sh/helm/v4@v4.2.4).
func (g ociHelmGenerator) registryClient(settings *cli.EnvSettings) (*registry.Client, error) {
	opts := []registry.ClientOption{
		registry.ClientOptDebug(settings.Debug),
		registry.ClientOptEnableCache(true),
		registry.ClientOptCredentialsFile(settings.RegistryConfig),
	}
	// helm itself only applies these credentials when both are present
	// (registry.Client.NewClient, helm.sh/helm/v4@v4.2.4 pkg/registry/client.go:127:
	// `if client.username != "" && client.password != ""`); a username-only or
	// password-only config falls through to the on-disk credentials store there
	// regardless of what ClientOptBasicAuth was given. Matching the condition
	// here, instead of the wider `||` this used to be, keeps that gate visible
	// in this file rather than only inside helm - it does not change what
	// NewClient does with a partial credential, just stops implying the
	// opposite.
	if username, password := credentials(g.config); username != "" && password != "" {
		opts = append(opts, registry.ClientOptBasicAuth(username, password))
	}
	return registry.NewClient(opts...)
}

func (g ociHelmGenerator) addRepoArgs(in []string) []string {
	return append(in, g.config.RepoUrl)
}

func newOciHelmGenerator(config *types.HelmChart) ociHelmGenerator {
	return ociHelmGenerator{config}
}
