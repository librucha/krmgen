package types

import (
	"text/template"
)

type Config struct {
	ApiVersion string    `yaml:"apiVersion"`
	Kind       string    `yaml:"kind"`
	Metadata   *Metadata `yaml:"metadata"`
	Helm       *Helm     `yaml:"helm"`
}

func (config Config) HasHelm() bool {
	return config.Helm != nil && config.Helm.Charts != nil && len(*config.Helm.Charts) > 0
}

type Metadata struct {
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

type Helm struct {
	Charts *[]HelmChart `yaml:"charts"`
}

type HelmChart struct {
	Name              string         `yaml:"name"`
	RepoUrl           string         `yaml:"repo"`
	IgnoreCredentials bool           `yaml:"ignoreCredentials"`
	Username          string         `yaml:"repoUser"`
	Password          string         `yaml:"repoPassword"`
	ReleaseName       string         `yaml:"releaseName"`
	Version           string         `yaml:"version"`
	ValuesInline      map[string]any `yaml:"valuesInline"`
	ValuesFile        string         `yaml:"valuesFile"`
}

type SecretFuncMap struct {
	template.FuncMap
}

type SecreteProvider interface {
	Provide(funcMap *SecretFuncMap)
}
