package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const schemaPath = "../../resources/krmgen-config-schema.json"

func loadSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compiling schema %s failed: %v", schemaPath, err)
	}
	return schema
}

// yamlToAny converts a YAML file to the generic structure the validator expects.
func yamlToAny(t *testing.T, path string) any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s failed: %v", path, err)
	}
	var doc any
	if err := yaml.Unmarshal(content, &doc); err != nil {
		t.Fatalf("unmarshaling %s failed: %v", path, err)
	}
	return doc
}

func TestSchemaAcceptsFixtures(t *testing.T) {
	fixtures := []string{
		"../../test/resources/full/full-krmgen-config.yaml",
		"../../test/resources/kustomization-only/krmgen.yaml",
	}
	schema := loadSchema(t)
	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			if err := schema.Validate(yamlToAny(t, fixture)); err != nil {
				t.Errorf("fixture %s should be valid but was rejected: %v", fixture, err)
			}
		})
	}
}

func TestSchemaRejectsInvalidConfigs(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "wrong kind",
			content: "kind: NotKrmGen\n",
		},
		{
			name:    "chart without name",
			content: "kind: KrmGen\nhelm:\n  charts:\n    - repo: oci://reg.io/helm/app\n",
		},
		{
			name:    "skip is not a list",
			content: "kind: KrmGen\nskip: \"*.pfx\"\n",
		},
		{
			name:    "ignoreCredentials is not a boolean",
			content: "kind: KrmGen\nhelm:\n  charts:\n    - name: app\n      ignoreCredentials: yes-please\n",
		},
	}
	schema := loadSchema(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc any
			if err := yaml.Unmarshal([]byte(tt.content), &doc); err != nil {
				t.Fatalf("unmarshaling test input failed: %v", err)
			}
			if err := schema.Validate(doc); err == nil {
				t.Errorf("config %q should be rejected but passed validation", tt.name)
			}
		})
	}
}
