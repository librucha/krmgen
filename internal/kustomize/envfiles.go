package kustomize

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// generatorFields are the kustomization fields whose entries can pull
// key/value pairs out of an env file.
var generatorFields = []string{"secretGenerator", "configMapGenerator"}

// dataKeyRegexp is the key format the Kubernetes API accepts in a ConfigMap or
// Secret. kustomize means to validate env-file keys as well - keyValuesFromLine
// calls Validator.IsEnvVarName on every one - but the implementation behind
// that call is an unimplemented stub returning nil
// (FieldValidator.IsEnvVarName, sigs.k8s.io/kustomize/api/internal/validate/
// fieldvalidator.go), so in practice any text at all becomes a key.
var dataKeyRegexp = regexp.MustCompile(`^[-._a-zA-Z0-9]+$`)

// utf8bom is stripped from an env file's first line, the way kustomize's own
// parser does.
var utf8bom = []byte{0xEF, 0xBB, 0xBF}

// checkGeneratorEnvFiles rejects a generator env file holding a line that
// cannot become a ConfigMap or Secret key.
//
// The .properties format env files use has no way to carry a newline. A value
// spanning several lines is truncated at the first line break, and every
// following line is read as a further key with an empty value. For a value
// krmgen pulled out of a key vault that is the worst of both outcomes: the
// credential is destroyed, and its remainder ends up as an unencoded key name
// that `kubectl get secret -o yaml` shows without so much as a base64 step.
// Nothing downstream catches it either - the API server would reject those
// keys, but only after the manifest has been produced and logged.
//
// Only the kustomization krmgen builds is checked. Env files reachable through
// a nested kustomization are resolved by kustomize itself and never pass
// through here.
func checkGeneratorEnvFiles(kustomization map[string]any, dir string) error {
	for _, field := range generatorFields {
		entries, ok := kustomization[field].([]any)
		if !ok {
			continue
		}
		for _, entry := range entries {
			generator, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			name, _ := generator["name"].(string)
			for _, ref := range envFileRefs(generator) {
				if err := checkEnvFile(filepath.Join(dir, ref), field, name, ref); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// envFileRefs collects the env sources of one generator entry: the "envs" list
// and the deprecated singular "env" string kustomize still accepts.
func envFileRefs(generator map[string]any) []string {
	var refs []string
	if single, ok := generator["env"].(string); ok && single != "" {
		refs = append(refs, single)
	}
	list, ok := generator["envs"].([]any)
	if !ok {
		return refs
	}
	for _, item := range list {
		if ref, ok := item.(string); ok {
			refs = append(refs, ref)
		}
	}
	return refs
}

func checkEnvFile(path, field, name, ref string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		// A missing or unreadable env file is kustomize's error to report,
		// and it words it better than this check could. This check exists
		// only to reject content kustomize accepts but should not.
		return nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(content))
	// scanner.Err() is deliberately not consulted: kustomize's own parser
	// ignores it too, so honouring it here would fail renders kustomize is
	// willing to perform.
	for number := 1; scanner.Scan(); number++ {
		key, ok := envFileKey(scanner.Bytes(), number == 1)
		if !ok || dataKeyRegexp.MatchString(key) {
			continue
		}
		// The offending line is a fragment of the value, so it is never
		// quoted back - naming the file and the line is enough to find it.
		return fmt.Errorf(
			"%s %q: env file %q line %d is read as a key, but it is not a valid ConfigMap/Secret key. "+
				"An env file cannot carry a value spanning multiple lines: the value is truncated at the "+
				"first line break and every following line becomes a key of its own. Write such a value to "+
				"its own file and reference it from the generator's \"files\" field instead of \"envs\"",
			field, name, ref, number)
	}
	return nil
}

// envFileKey extracts the key kustomize's keyValuesFromLine would read from
// one line, reporting false for a line it skips (blank or a comment).
func envFileKey(line []byte, first bool) (string, bool) {
	if first {
		line = bytes.TrimPrefix(line, utf8bom)
	}
	line = bytes.TrimLeftFunc(line, unicode.IsSpace)
	if len(line) == 0 || line[0] == '#' {
		return "", false
	}
	return strings.SplitN(string(line), "=", 2)[0], true
}
