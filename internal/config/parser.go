package config

import (
	types "github.com/librucha/krmgen/internal"
	"gopkg.in/yaml.v3"
	"log"
	"os"
	"path/filepath"
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
		return false
	}
	kind := contentObject["kind"]
	if kind == "KrmGen" {
		return true
	}
	return false
}

// ReadSkipPatterns scans srcDir (non-recursively) for KrmGen config files and
// returns their skip patterns without Go template evaluation. Used before copyDir.
func ReadSkipPatterns(srcDir string) []string {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil
	}
	var patterns []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(srcDir, entry.Name()))
		if err != nil {
			continue
		}
		var raw struct {
			Kind string   `yaml:"kind"`
			Skip []string `yaml:"skip"`
		}
		if err := yaml.Unmarshal(content, &raw); err != nil {
			continue
		}
		if raw.Kind == "KrmGen" {
			patterns = append(patterns, raw.Skip...)
		}
	}
	return patterns
}

func ParseConfig(filePath string) (*types.Config, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var config types.Config

	if err := yaml.Unmarshal(content, &config); err != nil {
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
