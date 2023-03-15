package config

import (
	types "github.com/librucha/krmgen/internal"
	"github.com/librucha/krmgen/internal/template"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"regexp"
)

var placeholderPattern = regexp.MustCompile(`\$\s*\{env:([^:]+):?(.*?)\}`)

func IsConfigFile(filePath string) bool {
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Println(err)
		return false
	}
	var contentObject map[string]any
	err = yaml.Unmarshal(content, &contentObject)
	if err != nil {
		log.Println(err)
		return false
	}
	kind := contentObject["kind"]
	if kind == "KrmGen" {
		return true
	}
	return false
}

func ParseConfig(filePath string) (*types.Config, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config types.Config

	evalContent, err := template.EvalGoTemplates(string(content))
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal([]byte(evalContent), &config); err != nil {
		return nil, err
	}

	// Validate by schema
	// compiler := jsonschema.NewCompiler()
	// compiler.Draft = jsonschema.Draft4
	// schema, err := compiler.Compile("../../resources/krmgen-config-schema.json")
	// if err != nil {
	//	log.Fatal(err)
	// }
	// if err := schema.Validate(config); err != nil {
	//	log.Fatal(err)
	// }

	return &config, nil
}
