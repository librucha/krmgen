# Template Values Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `values:` in `kind: KrmGen` files, evaluated leaf by leaf, exposed as `.Values` to every templated file.

**Architecture:** A new `config.ResolveValues(srcDir)` reads the raw YAML of top-level KrmGen files (sorted by name), walks `values` as a `yaml.Node` in document order and evaluates each string leaf with the values resolved so far. `cmd/generate.go` calls it after `ReadSkipPatterns` and passes `{"Values": values}` into `template.EvalGoTemplates` for every non-skipped file. The template engine runs with `missingkey=error`.

**Tech Stack:** Go, `text/template`, `gopkg.in/yaml.v3` (`yaml.Node`), sprig.

**Spec:** `docs/superpowers/specs/2026-10-08-template-values-design.md`

## Global Constraints

- Access syntax: `.Values.<key>`; `$.Values.<key>` inside `range` / `with`.
- Template data root is exactly `map[string]any{"Values": values}`.
- Templated values are quoted YAML strings; result of a templated leaf is always `string` (no YAML re-parse).
- Non-string scalars (int, float, bool, null) stay typed.
- Values of all top-level KrmGen files merged; duplicate top-level key → error `value "<key>" defined in both <fileA> and <fileB>`.
- `values` not a mapping → error `values in <file> must be a mapping`.
- Value template error → `evaluating value <path> in <file> failed error: <err>`; never print the evaluated result.
- Missing key → error (`missingkey=error`), never `<no value>`.
- Unparseable YAML file at top level is skipped by `ResolveValues`, like `ReadSkipPatterns`.
- Files matching `skip` are not evaluated.
- Comments, docs, commits in English; Conventional Commits; commit trailer `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Do not push.

## Review Focus

1. A file that already contains a stray `{{ .Something }}` used to render `<no value>`; now it fails with `map has no entry for key "Something"`. Expected: a clear error naming the file — pinned by `TestEvalGoTemplates_MissingKeyIsAnError` (Task 1).
2. Unquoted templated value (`keyvault: {{ ... }}`) makes `krmgen.yaml` unparseable, so values silently vanish. Expected: first `.Values.x` use fails and the message carries the quoting hint — pinned by `TestCopyDir_MissingValueErrorHasQuotingHint` (Task 3).
3. A nested value referencing its earlier sibling (`db.url` using `.Values.db.host`). Expected: works — pinned by the `nested sibling reference` case (Task 2).
4. Two KrmGen files where the later file (by name) references the earlier one's value. Expected: works; the reverse direction fails — pinned by `cross-file reference follows file name order` and `forward reference across files fails` (Task 2).
5. A non-KrmGen YAML at top level with its own `values:` key (e.g. a helm `values.yaml` that has a top-level `values:`). Expected: ignored — pinned by `non-KrmGen file is ignored` (Task 2).

---

### Task 1: Template engine accepts data and fails on missing keys

**Files:**
- Modify: `internal/template/template.go:92-109`
- Modify: `internal/template/template_test.go` (all `EvalGoTemplates(` calls, new tests)
- Modify: `cmd/generate.go:152` (pass `nil` for now)

**Interfaces:**
- Produces: `func EvalGoTemplates(content string, data any) (string, error)`

- [ ] **Step 1: Write the failing tests** — append to `internal/template/template_test.go`:

```go
func TestEvalGoTemplates_PassesData(t *testing.T) {
	data := map[string]any{"Values": map[string]any{"keyvault": "kv-prod", "zones": []any{"a", "b"}}}
	got, err := EvalGoTemplates(`{{ .Values.keyvault }}|{{ range .Values.zones }}{{ . }}-{{ $.Values.keyvault }};{{ end }}`, data)
	if err != nil {
		t.Fatal(err)
	}
	if want := "kv-prod|a-kv-prod;b-kv-prod;"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEvalGoTemplates_MissingKeyIsAnError(t *testing.T) {
	data := map[string]any{"Values": map[string]any{"keyvault": "kv-prod"}}
	_, err := EvalGoTemplates(`{{ .Values.keyvalut }}`, data)
	if err == nil {
		t.Fatal("expected a missing key to fail, it rendered instead")
	}
	if !strings.Contains(err.Error(), `map has no entry for key "keyvalut"`) {
		t.Errorf("error = %q, want it to name the missing key", err)
	}
}
```

In the same file replace every existing `EvalGoTemplates(x)` call with `EvalGoTemplates(x, nil)` (three call sites: lines ~107, ~141, ~180).

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/template/ -run 'TestEvalGoTemplates' -v`
Expected: compile error `too many arguments in call to EvalGoTemplates`.

- [ ] **Step 3: Implement** — in `internal/template/template.go` replace `EvalGoTemplates` with:

```go
// EvalGoTemplates evaluates content as a Go template with data as its root
// (".") - in practice {"Values": ...} built by config.ResolveValues.
// missingkey=error turns a typo such as .Values.keyvalut into an error
// instead of a silent "<no value>" in the rendered output.
func EvalGoTemplates(content string, data any) (string, error) {
	if goutils.IsBlank(content) {
		return content, nil
	}
	t := template.New("krmgen").Option("missingkey=error")
	if err := initFuncs(t); err != nil {
		return "", err
	}
	tmpl, err := t.Parse(content)
	if err != nil {
		return "", err
	}
	var buffer strings.Builder
	if err := tmpl.Execute(&buffer, data); err != nil {
		return "", err
	}
	return buffer.String(), nil
}
```

In `cmd/generate.go:152` change the call to `template.EvalGoTemplates(string(fileContent), nil)` (Task 3 replaces `nil`).

- [ ] **Step 4: Verify**

Run: `go build ./... && go test ./internal/template/... ./cmd/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/template/template.go internal/template/template_test.go cmd/generate.go
git commit -m "feat(template): pass data to templates and fail on missing keys"
```

---

### Task 2: `config.ResolveValues`

**Files:**
- Create: `internal/config/values.go`
- Create: `internal/config/values_test.go`

**Interfaces:**
- Consumes: `template.EvalGoTemplates(content string, data any) (string, error)` (Task 1)
- Produces: `func ResolveValues(srcDir string) (map[string]any, error)`; `const ValuesKey = "Values"` (data root key used by `cmd`)

- [ ] **Step 1: Write the failing tests** — `internal/config/values_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolveValues(t *testing.T) {
	t.Setenv("ARGOCD_ENV_CLUSTER_PROFILE", "prod")
	tests := []struct {
		name  string
		files map[string]string
		want  map[string]any
	}{
		{
			name: "chaining in document order",
			files: map[string]string{"krmgen.yaml": `kind: KrmGen
values:
  clusterProfile: '{{ argocdEnv "CLUSTER_PROFILE" }}'
  keyvault: '{{ printf "rixocz-%s-aks-vault" .Values.clusterProfile }}'
`},
			want: map[string]any{"clusterProfile": "prod", "keyvault": "rixocz-prod-aks-vault"},
		},
		{
			name: "non-string scalars stay typed, templated leaf is a string",
			files: map[string]string{"krmgen.yaml": `kind: KrmGen
values:
  replicas: 2
  ratio: 0.5
  enabled: true
  nothing: null
  rendered: '{{ 3 }}'
`},
			want: map[string]any{"replicas": 2, "ratio": 0.5, "enabled": true, "nothing": nil, "rendered": "3"},
		},
		{
			name: "nested sibling reference",
			files: map[string]string{"krmgen.yaml": `kind: KrmGen
values:
  db:
    host: db.internal
    url: '{{ printf "postgres://%s:5432" .Values.db.host }}'
`},
			want: map[string]any{"db": map[string]any{"host": "db.internal", "url": "postgres://db.internal:5432"}},
		},
		{
			name: "lists are walked",
			files: map[string]string{"krmgen.yaml": `kind: KrmGen
values:
  profile: prod
  zones: [a, '{{ .Values.profile }}-b', {name: '{{ .Values.profile }}'}]
`},
			want: map[string]any{"profile": "prod", "zones": []any{"a", "prod-b", map[string]any{"name": "prod"}}},
		},
		{
			name: "cross-file reference follows file name order",
			files: map[string]string{
				"a.yaml": "kind: KrmGen\nvalues:\n  base: kv\n",
				"b.yaml": "kind: KrmGen\nvalues:\n  full: '{{ .Values.base }}-prod'\n",
			},
			want: map[string]any{"base": "kv", "full": "kv-prod"},
		},
		{
			name: "non-KrmGen file is ignored",
			files: map[string]string{
				"krmgen.yaml": "kind: KrmGen\n",
				"values.yaml": "values:\n  x: '{{ .Values.missing }}'\n",
			},
			want: map[string]any{},
		},
		{
			name: "unparseable file is skipped",
			files: map[string]string{
				"krmgen.yaml": "kind: KrmGen\nvalues:\n  x: {{ argocdEnv \"CLUSTER_PROFILE\" }}\n",
			},
			want: map[string]any{},
		},
		{
			name:  "no values anywhere",
			files: map[string]string{"krmgen.yaml": "kind: KrmGen\n"},
			want:  map[string]any{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveValues(writeFiles(t, tt.files))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestResolveValues_Errors(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		wantErr []string
	}{
		{
			name:    "forward reference in one file fails",
			files:   map[string]string{"krmgen.yaml": "kind: KrmGen\nvalues:\n  a: '{{ .Values.b }}'\n  b: x\n"},
			wantErr: []string{"evaluating value values.a in krmgen.yaml failed", `map has no entry for key "b"`},
		},
		{
			name: "forward reference across files fails",
			files: map[string]string{
				"a.yaml": "kind: KrmGen\nvalues:\n  full: '{{ .Values.base }}-prod'\n",
				"b.yaml": "kind: KrmGen\nvalues:\n  base: kv\n",
			},
			wantErr: []string{"evaluating value values.full in a.yaml failed"},
		},
		{
			name: "duplicate key across files",
			files: map[string]string{
				"a.yaml":      "kind: KrmGen\nvalues:\n  keyvault: one\n",
				"krmgen.yaml": "kind: KrmGen\nvalues:\n  keyvault: two\n",
			},
			wantErr: []string{`value "keyvault" defined in both a.yaml and krmgen.yaml`},
		},
		{
			name:    "values not a mapping",
			files:   map[string]string{"krmgen.yaml": "kind: KrmGen\nvalues: [a, b]\n"},
			wantErr: []string{"values in krmgen.yaml must be a mapping"},
		},
		{
			name:    "nested path is named",
			files:   map[string]string{"krmgen.yaml": "kind: KrmGen\nvalues:\n  db:\n    url: '{{ .Values.nope }}'\n"},
			wantErr: []string{"evaluating value values.db.url in krmgen.yaml failed"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveValues(writeFiles(t, tt.files))
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

func TestResolveValues_ErrorDoesNotLeakResolvedValues(t *testing.T) {
	dir := writeFiles(t, map[string]string{"krmgen.yaml": "kind: KrmGen\nvalues:\n  secret: s3cr3t-value\n  broken: '{{ .Values.secret }}{{ .Values.nope }}'\n"})
	_, err := ResolveValues(dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "s3cr3t-value") {
		t.Errorf("error leaks a resolved value: %q", err)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/ -run TestResolveValues -v`
Expected: compile error `undefined: ResolveValues`.

- [ ] **Step 3: Implement** — `internal/config/values.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"

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
		if !entry.Type().IsRegular() {
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
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if owner, ok := owners[key]; ok {
				return nil, fmt.Errorf("value %q defined in both %s and %s", key, owner, file)
			}
			owners[key] = file
			if err := resolveInto(values, key, node.Content[i+1], "values."+key, file, data); err != nil {
				return nil, err
			}
		}
	}
	return values, nil
}

// readValuesNode returns the values node of a KrmGen file, nil when the file
// is not a KrmGen file or has no values, and an error when it does not parse.
func readValuesNode(path string) (*yaml.Node, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, nil
	}
	root := doc.Content[0]
	var kind string
	var values *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "kind":
			kind = root.Content[i+1].Value
		case "values":
			values = root.Content[i+1]
		}
	}
	if kind != "KrmGen" {
		return nil, nil
	}
	return values, nil
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
		for i := 0; i+1 < len(node.Content); i += 2 {
			childKey := node.Content[i].Value
			if err := resolveInto(nested, childKey, node.Content[i+1], path+"."+childKey, file, data); err != nil {
				return err
			}
		}
		return nil
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
		for i := 0; i+1 < len(node.Content); i += 2 {
			childKey := node.Content[i].Value
			if err := resolveInto(nested, childKey, node.Content[i+1], path+"."+childKey, file, data); err != nil {
				return nil, err
			}
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
```

- [ ] **Step 4: Verify**

Run: `go test ./internal/config/ -run TestResolveValues -v && go vet ./internal/config/`
Expected: PASS. If the import of `internal/template` creates a cycle, stop and report — none exists today (`grep -rn 'internal/template"' internal` is empty).

- [ ] **Step 5: Commit**

```bash
git add internal/config/values.go internal/config/values_test.go
git commit -m "feat(config): resolve values from KrmGen files in document order"
```

---

### Task 3: Wire values into `generate`, type, schema, fixture

**Files:**
- Modify: `cmd/generate.go` (`generate`, `copySrcDir`, `copyDir`)
- Modify: `cmd/generate_test.go` (call sites of `copySrcDir`/`copyDir`, new tests)
- Modify: `internal/types.go` (`Config.Values`)
- Modify: `resources/krmgen-config-schema.json` (`values`)
- Modify: `internal/config/schema_test.go` (fixture list, rejection case)
- Create: `test/resources/values/krmgen.yaml`, `test/resources/values/kustomization.yaml`, `test/resources/values/manifests/configmap.yaml`, `test/resources/values/raw.txt`

**Interfaces:**
- Consumes: `config.ResolveValues(srcDir string) (map[string]any, error)`, `config.ValuesKey` (Task 2); `template.EvalGoTemplates(content string, data any)` (Task 1)
- Produces: `copySrcDir(srcDir string, skipPatterns []string, data map[string]any) (string, error)`; `copyDir(srcDir, dstDir, baseDir string, skipPatterns []string, data map[string]any) error`

- [ ] **Step 1: Create the fixture**

`test/resources/values/krmgen.yaml`:
```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
skip:
  - "*.txt"
values:
  clusterProfile: '{{ argocdEnv "CLUSTER_PROFILE" }}'
  keyvault: '{{ printf "rixocz-%s-aks-vault" .Values.clusterProfile }}'
  replicas: 2
  db:
    host: '{{ printf "db.%s.internal" .Values.clusterProfile }}'
    url: '{{ printf "postgres://%s:5432" .Values.db.host }}'
  zones:
    - a
    - b
```

`test/resources/values/kustomization.yaml`:
```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
namespace: '{{ .Values.clusterProfile }}'
resources:
  - manifests/configmap.yaml
```

`test/resources/values/manifests/configmap.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app
data:
  keyvault: '{{ .Values.keyvault }}'
  replicas: '{{ .Values.replicas }}'
  dbUrl: '{{ .Values.db.url }}'
  zones: '{{ range .Values.zones }}[{{ . }}@{{ $.Values.clusterProfile }}]{{ end }}'
```

`test/resources/values/raw.txt`:
```
{{ .Values.keyvault }} stays literal
```

- [ ] **Step 2: Write the failing tests** — in `cmd/generate_test.go`, update existing calls: `copySrcDir(x, y)` → `copySrcDir(x, y, nil)`, `copyDir(a, b, c, d)` → `copyDir(a, b, c, d, nil)`. Append:

```go
func TestGenerate_ValuesReachEveryTemplatedFile(t *testing.T) {
	t.Setenv("ARGOCD_ENV_CLUSTER_PROFILE", "prod")
	src, err := filepath.Abs("../test/resources/values")
	if err != nil {
		t.Fatal(err)
	}

	stdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	genErr := generate(src, mergeSkipPatterns(config.ReadSkipPatterns(src), nil))
	_ = w.Close()
	os.Stdout = stdout
	if genErr != nil {
		t.Fatal(genErr)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"namespace: prod",
		"keyvault: rixocz-prod-aks-vault",
		`replicas: "2"`,
		"dbUrl: postgres://db.prod.internal:5432",
		"[a@prod][b@prod]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

func TestCopySrcDir_SkippedFileGetsNoValues(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "raw.txt"), []byte("{{ .Values.keyvault }}"), 0600); err != nil {
		t.Fatal(err)
	}
	workDir, err := copySrcDir(src, []string{"*.txt"}, map[string]any{"Values": map[string]any{"keyvault": "kv"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })
	got, err := os.ReadFile(filepath.Join(workDir, "raw.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "{{ .Values.keyvault }}" {
		t.Errorf("skipped file was evaluated: %q", got)
	}
}

func TestCopyDir_MissingValueErrorHasQuotingHint(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "secret.yaml"), []byte("x: {{ .Values.keyvault }}"), 0600); err != nil {
		t.Fatal(err)
	}
	err := copyDir(src, t.TempDir(), src, nil, map[string]any{"Values": map[string]any{}})
	if err == nil {
		t.Fatal("expected a missing value to fail")
	}
	for _, want := range []string{"template evaluation of file", `map has no entry for key "keyvault"`, "quote templated values"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestGenerate_ValueErrorFailsTheRun(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "krmgen.yaml"), []byte("kind: KrmGen\nvalues:\n  a: '{{ .Values.b }}'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := generate(src, nil)
	if err == nil || !strings.Contains(err.Error(), "evaluating value values.a in krmgen.yaml failed") {
		t.Errorf("err = %v, want the value error", err)
	}
}
```

Add `"github.com/librucha/krmgen/internal/config"` to the test imports.

- [ ] **Step 3: Run to verify failure**

Run: `go test ./cmd/ -v -run 'Values|SkippedFile|QuotingHint'`
Expected: compile errors (`too many arguments in call to copySrcDir`).

- [ ] **Step 4: Implement** — `cmd/generate.go`:

`generate` resolves values before copying:
```go
func generate(srcDir string, skipPatterns []string) (err error) {
	values, err := config.ResolveValues(srcDir)
	if err != nil {
		return err
	}
	data := map[string]any{config.ValuesKey: values}
	workDir, err := copySrcDir(srcDir, skipPatterns, data)
	// ... rest unchanged
```

Thread `data map[string]any` through `copySrcDir` → `copyDir` (also the recursive call). Replace the evaluate branch with:
```go
			// evaluate templates
			evaluated, err := template.EvalGoTemplates(string(fileContent), data)
			if err != nil {
				return fmt.Errorf("template evaluation of file %s failed error: %w%s", srcPath, err, missingValueHint(err))
			}
```

and add:
```go
// missingValueHint explains the usual cause of a missing .Values key: a
// templated value left unquoted makes krmgen.yaml invalid YAML before
// templating, so ResolveValues skips the file and its values never exist.
func missingValueHint(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "map has no entry for key") && strings.Contains(msg, ".Values") {
		return " (values are read from krmgen.yaml before templating - quote templated values)"
	}
	return ""
}
```
Add `"strings"` to the imports.

`internal/types.go` — add to `Config` after `Skip`:
```go
	// Values is informational: the values templates see are resolved by
	// config.ResolveValues from the raw file, before templating.
	Values map[string]any `yaml:"values"`
```

`resources/krmgen-config-schema.json` — add after `skip`:
```json
    "values": {
      "type": "object",
      "description": "Named values exposed to every templated file as .Values. String leaves are Go templates evaluated in document order; quote them."
    },
```

`internal/config/schema_test.go` — add `"../../test/resources/values/krmgen.yaml"` to `fixtures`, and a rejection case:
```go
		{
			name:    "values is not an object",
			content: "kind: KrmGen\nvalues: [a, b]\n",
		},
```

- [ ] **Step 5: Verify**

Run: `go test ./cmd/ ./internal/config/ ./internal/template/ && go vet ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/generate.go cmd/generate_test.go internal/types.go resources/krmgen-config-schema.json internal/config/schema_test.go test/resources/values
git commit -m "feat: expose KrmGen values to every templated file as .Values"
```

---

### Task 4: Documentation

**Files:**
- Modify: `docs/specification.md` (Fields table ~line 210, Order of operations ~line 250, new section after `### Skip pattern matching`, Example in §2)
- Modify: `README.md` (`krmgen.yaml` reference ~line 164, new example after `### ArgoCD pipeline with dynamic values` ~line 374, TOC)
- Modify: `CLAUDE.md` (architecture block, `cmd/generate.go` steps; new `config/values.go` line)

- [ ] **Step 1: `docs/specification.md`**

Fields table — add after the `skip` row:
```markdown
| `values` | map | no | Named values exposed to every templated file as `.Values`; see Rendering pipeline, Values |
```

Order of operations — insert after step 2 (renumber the rest):
```markdown
3. Resolve `values` from every `kind: KrmGen` file at the top level of the
   source directory (`config.ResolveValues`, `internal/config/values.go`), on
   the **raw YAML**, before any file is copied. See Values below.
```
and in the copy step add: "Each evaluated file gets `{"Values": <resolved values>}` as its template data."

New section after `### Skip pattern matching`:
```markdown
### Values

`values:` declares named values, exposed to every templated file — including
`krmgen.yaml` itself, files in subdirectories and `kustomization.yaml` — as
`.Values.<key>` (`$.Values.<key>` inside `range` / `with`).

```yaml
kind: KrmGen
values:
  clusterProfile: '{{ argocdEnv "CLUSTER_PROFILE" }}'
  keyvault: '{{ printf "rixocz-%s-aks-vault" .Values.clusterProfile }}'
```

- Read from the raw YAML before templating: a templated value **must be
  quoted**. An unquoted `{{` makes the file invalid YAML; it is then skipped
  silently and its values surface as a missing-key error at first use.
- Every string leaf is a Go template, evaluated in document order with the
  values resolved before it in scope — including earlier siblings in the same
  nested map. Forward references fail.
- A templated leaf always yields a string (`'{{ 3 }}'` is `"3"`); non-string
  scalars (`replicas: 2`) stay typed. Maps and lists are walked recursively.
- Values of all top-level KrmGen files are merged, files in name order. The
  same top-level key in two files is an error.
- `values` that is not a map is an error.
- A reference to a key that does not exist is an error
  (`missingkey=error`), never `<no value>`. This applies to every template:
  a stray `{{ .Foo }}` that used to render `<no value>` now fails the run.
- A value error names the key path and file, never the evaluated result.
- Files matching a skip pattern are not evaluated and see no values.
```

- [ ] **Step 2: `README.md`**

In the `krmgen.yaml` reference block add a `values:` entry with a comment `# exposed as .Values in every templated file; quote templated values`. Add example section after `### ArgoCD pipeline with dynamic values`:

```markdown
### Shared values

```yaml
# krmgen.yaml
kind: KrmGen
values:
  clusterProfile: '{{ argocdEnv "CLUSTER_PROFILE" }}'
  keyvault: '{{ printf "rixocz-%s-aks-vault" .Values.clusterProfile }}'
```

```yaml
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: db
stringData:
  password: '{{ azSec .Values.keyvault "db-secret" }}'
```

Values are evaluated top to bottom, each seeing the ones above it. Quote
templated values: `krmgen.yaml` is read as YAML before templating.
```

Add the TOC entry `- [Shared values](#shared-values)` in the Examples list.

- [ ] **Step 3: `CLAUDE.md`**

Architecture block — `cmd/generate.go` steps become:
```
                      1. read skip patterns from krmgen.yaml (pre-copy, raw YAML)
                      2. resolve values from krmgen.yaml (pre-copy, raw YAML)
                      3. copy src dir to temp dir (evaluating Go templates with .Values in all files
                         except those matching skip patterns — copied as-is)
                      4. find KrmGen config files (kind: KrmGen)
                      5. ProcessConfig → helm + kustomize → stdout
```
Under `internal/` add after `config/processor.go`:
```
  config/values.go      → ResolveValues: values: of KrmGen files → .Values template data
```
Add a short "Values" subsection after "Skipping template evaluation" with the two-file example from README.

- [ ] **Step 4: Verify**

Run: `task check` (or `go test ./... && go vet ./...` when golden tests fail on local helm version drift — that failure predates this work; report it, do not fix it).
Expected: everything except the known golden drift passes.

- [ ] **Step 5: Commit**

```bash
git add docs/specification.md README.md CLAUDE.md
git commit -m "docs: document template values"
```
