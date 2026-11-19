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
	for i, assignment := range vars {
		if err := setVar(values, assignment); err != nil {
			// Named by position: until the key is known to be well-formed
			// there is nothing else safe to quote (see setVar).
			return nil, fmt.Errorf("invalid --var #%d: %w", i+1, err)
		}
	}
	return values, nil
}

// readVarFile decodes a .yaml, .yml or .json file into a mapping. All three
// go through the YAML parser on purpose: encoding/json would decode every
// number as float64, which a template prints as 1e+06 for 1000000, while this
// way a value gets the type it would have under values: in a KrmGen file. The
// price is on the JSON side: a .json file holding YAML-only syntax is not
// rejected, and the few JSON constructs YAML does not share - the \/ escape,
// a 😀 surrogate pair - are.
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
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, fmt.Errorf("parsing variables file %s failed error: %w", path, err)
	}
	if len(doc.Content) == 0 {
		// An empty file, or one holding only comments, declares nothing.
		return map[string]any{}, nil
	}
	root := doc.Content[0]
	if root.Kind == yaml.AliasNode {
		root = root.Alias
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("variables file %s must hold a mapping at the top level", path)
	}
	values, err := varMapping(root, "")
	if err != nil {
		return nil, fmt.Errorf("variables file %s: %w", path, err)
	}
	return values, nil
}

// varNode converts a YAML node into the plain Go value a template works
// with. It walks the node tree instead of unmarshalling into an any, because
// the generic decoder turns a mapping with a non-string key - ports: {80:
// http} - into a map[any]any: {{ index .Values.ports "80" }} then finds
// nothing and sprig's map functions refuse the type. Under values: the same
// mapping is a map[string]any keyed by the text of each key, and a variables
// file has to behave the same for an expression to be portable between the
// two.
//
// An alias is expanded into its own copy, so a --var written into one
// mapping never shows up under the anchor it came from.
func varNode(node *yaml.Node, path string) (any, error) {
	switch node.Kind {
	case yaml.AliasNode:
		return varNode(node.Alias, path)
	case yaml.MappingNode:
		return varMapping(node, path)
	case yaml.SequenceNode:
		list := make([]any, 0, len(node.Content))
		for i, item := range node.Content {
			value, err := varNode(item, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		return list, nil
	}
	var value any
	if err := node.Decode(&value); err != nil {
		// Name the key, never the content - a variable may hold a secret.
		return nil, fmt.Errorf("decoding %s failed", path)
	}
	return value, nil
}

// varMapping converts a mapping node. The checks mirror eachPair, which does
// the same job for values: - a yaml.Node keeps what the generic decoder would
// have rejected or resolved on its own: a repeated key (the last would
// silently win) and a merge key (its entries would become one literal "<<"
// value).
func varMapping(node *yaml.Node, path string) (map[string]any, error) {
	mapping := make(map[string]any, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s has a key that is not a scalar", pathOrTopLevel(path))
		}
		if keyNode.ShortTag() == "!!merge" {
			return nil, fmt.Errorf("%s uses a YAML merge key, which is not supported", pathOrTopLevel(path))
		}
		key := keyNode.Value
		childPath := key
		if path != "" {
			childPath = path + "." + key
		}
		if _, seen := mapping[key]; seen {
			return nil, fmt.Errorf("%s is defined twice", childPath)
		}
		value, err := varNode(node.Content[i+1], childPath)
		if err != nil {
			return nil, err
		}
		mapping[key] = value
	}
	return mapping, nil
}

func pathOrTopLevel(path string) string {
	if path == "" {
		return "the top level"
	}
	return path
}

// setVar applies one key=value assignment to values. The value is always a
// string. A dotted key addresses a nested mapping, creating the levels that
// are missing and leaving the siblings of the ones that exist alone.
//
// A level that exists but is not a mapping is an error rather than being
// replaced: with db: prod-db loaded from the file, db.host=x would otherwise
// quietly make {{ .Values.db }} print map[host:x]. A null level (db: with
// nothing below it) holds nothing worth protecting and is filled in.
//
// No error quotes the value: it may be a secret, and stderr ends up in logs.
// An assignment without "=" is not quoted at all, because nothing tells its
// key from its value; the caller names it by position instead.
func setVar(values map[string]any, assignment string) error {
	key, value, found := strings.Cut(assignment, "=")
	if !found {
		return fmt.Errorf("expected key=value")
	}
	if key == "" {
		return fmt.Errorf("the key before = is empty")
	}
	segments := strings.Split(key, ".")
	for _, segment := range segments {
		if segment == "" {
			return fmt.Errorf("key %q has an empty name between its dots", key)
		}
	}
	target := values
	for i, segment := range segments[:len(segments)-1] {
		child := target[segment]
		if child == nil {
			created := map[string]any{}
			target[segment] = created
			target = created
			continue
		}
		mapping, ok := child.(map[string]any)
		if !ok {
			return fmt.Errorf("key %q: %s is already set and is not a mapping", key, strings.Join(segments[:i+1], "."))
		}
		target = mapping
	}
	target[segments[len(segments)-1]] = value
	return nil
}
