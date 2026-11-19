package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadVars builds the .Values map of the template command: the mapping read
// from varFile (none when varFile is empty), with each key=value of vars
// applied on top in order, so a var overrides the file and a later var
// overrides an earlier one. The result is never nil - a nil .Values would
// turn a missing key into a nil-pointer error instead of a missing-key one.
//
// Unlike values: in a KrmGen file (ResolveValues), nothing here is evaluated
// as a template: a value is taken literally.
func LoadVars(varFile string, vars []string) (map[string]any, error) {
	values := map[string]any{}
	if varFile != "" {
		fromFile, err := readVarFile(varFile)
		if err != nil {
			return nil, err
		}
		values = fromFile
	}
	for _, assignment := range vars {
		if err := setVar(values, assignment); err != nil {
			return nil, err
		}
	}
	return values, nil
}

// readVarFile decodes a .yaml, .yml or .json file into a mapping. All three
// go through the YAML decoder on purpose: encoding/json would decode every
// number as float64, which a template prints as 1e+06 for 1000000, while the
// YAML decoder yields the same types values: in a KrmGen file has. The price
// is that a .json file holding YAML-only syntax is not rejected.
func readVarFile(path string) (map[string]any, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml", ".json":
	default:
		return nil, fmt.Errorf("variables file %s has an unsupported extension, expected .yaml, .yml or .json", path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading variables file %s failed error: %w", path, err)
	}
	var decoded any
	if err := yaml.Unmarshal(content, &decoded); err != nil {
		return nil, fmt.Errorf("parsing variables file %s failed error: %w", path, err)
	}
	switch typed := decoded.(type) {
	case nil:
		// An empty file, or one holding only comments, declares nothing.
		return map[string]any{}, nil
	case map[string]any:
		return typed, nil
	default:
		return nil, fmt.Errorf("variables file %s must hold a mapping with string keys at the top level", path)
	}
}

// setVar applies one key=value assignment to values. The value is always a
// string. A dotted key addresses a nested mapping, creating the levels that
// are missing and leaving the siblings of the ones that exist alone.
//
// A level that exists but is not a mapping is an error rather than being
// replaced: with db: prod-db loaded from the file, db.host=x would otherwise
// quietly make {{ .Values.db }} print map[host:x].
func setVar(values map[string]any, assignment string) error {
	key, value, found := strings.Cut(assignment, "=")
	if !found {
		return fmt.Errorf("invalid --var %q: expected key=value", assignment)
	}
	segments := strings.Split(key, ".")
	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf("invalid --var %q: key %q has an empty name", assignment, key)
		}
	}
	target := values
	for i, segment := range segments[:len(segments)-1] {
		child, exists := target[segment]
		if !exists {
			created := map[string]any{}
			target[segment] = created
			target = created
			continue
		}
		mapping, ok := child.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid --var %q: %s is already set and is not a mapping", assignment, strings.Join(segments[:i+1], "."))
		}
		target = mapping
	}
	target[segments[len(segments)-1]] = value
	return nil
}
