package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/librucha/krmgen/internal/template"
	"gopkg.in/yaml.v3"
)

// ValuesKey is the template data key values are exposed under: .Values.
const ValuesKey = "Values"

// ResolveValues collects the values: mappings of every top-level kind: KrmGen
// file in srcDir and evaluates them. It reads the raw YAML, before any
// template runs, so a templated value has to be a quoted string. Files are
// taken in name order (os.ReadDir) and keys in document order; every string
// leaf is a template evaluated with the values resolved before it. A file that
// does not parse is skipped, as in ReadSkipPatterns - its values then surface
// as a missing-key error at their first use.
func ResolveValues(srcDir string) (map[string]any, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("reading source directory %s failed error: %w", srcDir, err)
	}
	values := map[string]any{}
	data := map[string]any{ValuesKey: values}
	owners := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		node, err := readValuesNode(filepath.Join(srcDir, entry.Name()))
		if err != nil || node == nil {
			continue
		}
		file := entry.Name()
		if node.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("values in %s must be a mapping", file)
		}
		err = eachPair(node, "values", file, func(key string, child *yaml.Node, path string) error {
			if owner, ok := owners[key]; ok {
				return fmt.Errorf("value %q defined in both %s and %s", key, owner, file)
			}
			owners[key] = file
			return resolveInto(values, key, child, path, file, data)
		})
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}

// StripValues blanks the values block of a top-level KrmGen file before the
// file itself is templated: values are evaluated once, by ResolveValues, and
// a second rendering of the raw block could break its YAML - a resolved value
// holding a quote ends a single-quoted scalar early. Every line from the
// values key up to the next top-level key, the next document or EOF becomes
// empty, so the line count - and with it any template error position - stays
// the same. Content that does not parse as a KrmGen mapping with values is
// returned unchanged.
func StripValues(content []byte) []byte {
	root, keyIndex, err := parseKrmGen(content)
	if err != nil || root == nil || keyIndex < 0 {
		return content
	}
	lines := strings.Split(string(content), "\n")
	from := root.Content[keyIndex].Line - 1
	to := len(lines)
	if next := keyIndex + 2; next < len(root.Content) {
		to = root.Content[next].Line - 1
	} else {
		for i := from + 1; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "---") || strings.HasPrefix(lines[i], "...") {
				to = i
				break
			}
		}
	}
	for i := from; i < to && i < len(lines); i++ {
		lines[i] = ""
	}
	return []byte(strings.Join(lines, "\n"))
}

// readValuesNode returns the values node of a KrmGen file, nil when the file
// is not a KrmGen file or has no values, and an error when it does not parse.
func readValuesNode(path string) (*yaml.Node, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	root, keyIndex, err := parseKrmGen(content)
	if err != nil || root == nil || keyIndex < 0 {
		return nil, err
	}
	return root.Content[keyIndex+1], nil
}

// parseKrmGen returns the root mapping of a kind: KrmGen document and the
// index of its values key in root.Content (-1 without values). root is nil
// when the content is not a KrmGen mapping; err is set when it does not parse.
func parseKrmGen(content []byte) (*yaml.Node, int, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, -1, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, -1, nil
	}
	root := doc.Content[0]
	var kind string
	keyIndex := -1
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "kind":
			kind = root.Content[i+1].Value
		case "values":
			keyIndex = i
		}
	}
	if kind != "KrmGen" {
		return nil, -1, nil
	}
	return root, keyIndex, nil
}

// resolveInto stores the resolved node under target[key]. A mapping is
// attached to target before its entries are resolved, so a later entry can
// reference an earlier sibling (.Values.db.host from .Values.db.url).
func resolveInto(target map[string]any, key string, node *yaml.Node, path, file string, data map[string]any) error {
	if node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node.Kind == yaml.MappingNode {
		nested := map[string]any{}
		target[key] = nested
		return resolveMapping(nested, node, path, file, data)
	}
	value, err := resolveNode(node, path, file, data)
	if err != nil {
		return err
	}
	target[key] = value
	return nil
}

func resolveNode(node *yaml.Node, path, file string, data map[string]any) (any, error) {
	switch node.Kind {
	case yaml.AliasNode:
		return resolveNode(node.Alias, path, file, data)
	case yaml.MappingNode:
		nested := map[string]any{}
		if err := resolveMapping(nested, node, path, file, data); err != nil {
			return nil, err
		}
		return nested, nil
	case yaml.SequenceNode:
		list := make([]any, 0, len(node.Content))
		for i, item := range node.Content {
			value, err := resolveNode(item, fmt.Sprintf("%s[%d]", path, i), file, data)
			if err != nil {
				return nil, err
			}
			list = append(list, value)
		}
		return list, nil
	}
	if node.ShortTag() == "!!str" {
		// Name the key, never the rendered result - values may hold secrets.
		evaluated, err := template.EvalGoTemplates(node.Value, data)
		if err != nil {
			return nil, fmt.Errorf("evaluating value %s in %s failed error: %w", path, file, err)
		}
		return evaluated, nil
	}
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, fmt.Errorf("decoding value %s in %s failed error: %w", path, file, err)
	}
	return value, nil
}

// resolveMapping resolves every entry of a mapping node into target.
func resolveMapping(target map[string]any, node *yaml.Node, path, file string, data map[string]any) error {
	return eachPair(node, path, file, func(key string, child *yaml.Node, childPath string) error {
		return resolveInto(target, key, child, childPath, file, data)
	})
}

// eachPair calls fn for every key/value pair of a mapping node in document
// order. It is the one place that rejects a non-scalar key: an unquoted
// {{ ... }} parses as a flow mapping used as a key, which would otherwise
// silently become a bogus value. It also rejects a key repeated inside one
// mapping - yaml.Node keeps both, and the last would silently win - and a
// merge key (<<), whose entries would otherwise become a literal "<<" value.
func eachPair(node *yaml.Node, path, file string, fn func(key string, value *yaml.Node, childPath string) error) error {
	seen := map[string]bool{}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode.Kind != yaml.ScalarNode {
			return fmt.Errorf("values in %s: %s has a non-string key - quote templated values", file, path)
		}
		if keyNode.ShortTag() == "!!merge" {
			return fmt.Errorf("values in %s: %s uses a YAML merge key, which is not supported", file, path)
		}
		childPath := path + "." + keyNode.Value
		if seen[keyNode.Value] {
			return fmt.Errorf("value %s defined twice in %s", childPath, file)
		}
		seen[keyNode.Value] = true
		if err := fn(keyNode.Value, node.Content[i+1], childPath); err != nil {
			return err
		}
	}
	return nil
}
