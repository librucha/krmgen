# `krmgen template` Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `krmgen template <template>`, which evaluates one Go template string with krmgen's template functions and prints the result, with variables from `--var-file` and repeatable `--var`.

**Architecture:** A loader in `internal/config/vars.go` turns the flag values into a `map[string]any`. A cobra command in `cmd/template.go` passes that map under `.Values` to the existing `template.EvalGoTemplates` and prints the result. Nothing in the `generate` path changes.

**Tech Stack:** Go 1.27, `github.com/spf13/cobra`, `gopkg.in/yaml.v3`, standard `testing`.

**Spec:** `docs/superpowers/specs/2026-11-19-template-command-design.md`

## Global Constraints

- Evaluation goes through `template.EvalGoTemplates` only; no second engine or function set.
- `krmgen generate` output and behaviour do not change.
- `--var` is registered with `StringArrayVar`, never `StringSliceVar` (the slice variant splits values on commas).
- `--var-file` accepts `.yaml`, `.yml`, `.json`, matched case-insensitively; all three are parsed with `gopkg.in/yaml.v3`.
- `--var` values are always strings and are never evaluated as templates.
- Output is the evaluated text exactly, with no newline added.
- Production code returns errors; no `log.Fatal`. Errors leaving the command are wrapped in `redact.Error`.
- All comments in English.
- `docs/dockerhub.md` must stay free of `{{`.
- Commits use Conventional Commits and end with `Co-Authored-By: Claude Fable 5.2 <noreply@anthropic.com>`. Do not push.

## Review Focus

- A template argument that starts with `-` (for example `'-{{ .Values.x }}'`) is read by cobra as a flag. `krmgen template -- '-x'` must print `-x`. Test in Task 2.
- A `--var` value containing a comma, `=`, or spaces must arrive unchanged: `--var hosts=a,b`, `--var q=a=b`, `--var ns='black widow'`. Tests in Tasks 1 and 2.
- Numbers from a `.json` file must keep their written form: `{"n": 1000000}` prints `1000000`, not `1e+06`. Test in Task 1 (type) and Task 2 (printed).
- A dotted `--var` must keep the sibling keys of the mapping it writes into: file `db: {host: a, port: 5432}` plus `--var db.host=b` leaves `port` at `5432`. Test in Task 1.
- An empty template argument (`''`) must exit 0 and print nothing. Test in Task 2.

---

### Task 1: Variable loader

**Files:**
- Create: `internal/config/vars.go`
- Test: `internal/config/vars_test.go`

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `func LoadVars(varFile string, vars []string) (map[string]any, error)` in package `config`. `varFile == ""` means no file. The result is never nil. Nested mappings are `map[string]any`.

- [ ] **Step 1: Write the failing tests** in `internal/config/vars_test.go` (package `config`). Files are written into `t.TempDir()`.

`TestLoadVars_File`, table-driven, one sub-test per row; compare with `reflect.DeepEqual`:

| name | file name | content | want |
|---|---|---|---|
| yaml | `v.yaml` | `ns: black widow\nreplicas: 3\n` | `{"ns": "black widow", "replicas": 3}` |
| yml | `v.yml` | `db:\n  host: a\n  port: 5432\n` | `{"db": {"host": "a", "port": 5432}}` |
| json | `v.json` | `{"n": 1000000, "tags": ["a", "b"], "on": true}` | `{"n": 1000000, "tags": []any{"a", "b"}, "on": true}` |
| upper-case extension | `v.YAML` | `a: b\n` | `{"a": "b"}` |
| empty file | `v.yaml` | `` | `{}` |

`replicas`, `port` and `n` must be Go `int`.

`TestLoadVars_FileErrors`, each must return a non-nil error whose text contains the file path:
- extension `.txt` with content `a: b` — error text also contains `.yaml`, `.yml` and `.json`
- no extension (`vars`)
- top-level sequence: `v.yaml` with `- a\n- b\n` — error text contains `mapping`
- top-level scalar: `v.yaml` with `just text\n`
- invalid YAML: `v.yaml` with `a: [1, 2\n`
- file does not exist

`TestLoadVars_NoFileNoVars`: `LoadVars("", nil)` returns an empty, non-nil map and no error.

`TestLoadVars_Var`, table-driven; `file` is the content of a `v.yaml` (empty string means no file):

| name | file | vars | want |
|---|---|---|---|
| plain | | `ns=black widow` | `{"ns": "black widow"}` |
| always a string | | `n=3`, `b=true` | `{"n": "3", "b": "true"}` |
| empty value | | `x=` | `{"x": ""}` |
| value with equals | | `q=a=b` | `{"q": "a=b"}` |
| value with comma | | `hosts=a,b` | `{"hosts": "a,b"}` |
| overrides file | `ns: one\n` | `ns=two` | `{"ns": "two"}` |
| later wins | | `ns=one`, `ns=two` | `{"ns": "two"}` |
| nested | | `db.host=x` | `{"db": {"host": "x"}}` |
| nested keeps siblings | `db:\n  host: a\n  port: 5432\n` | `db.host=b` | `{"db": {"host": "b", "port": 5432}}` |
| deep | | `a.b.c=x` | `{"a": {"b": {"c": "x"}}}` |
| plain replaces mapping | `db:\n  host: a\n` | `db=x` | `{"db": "x"}` |
| leaf replaces mapping | `a:\n  b:\n    c: 1\n` | `a.b=x` | `{"a": {"b": "x"}}` |

`TestLoadVars_VarErrors`, each must return a non-nil error whose text contains the offending `--var` argument:
- `ns` (no `=`)
- `=x` (empty key)
- `a..b=x`, `.a=x`, `a.=x` (empty path segment)
- file `db: prod-db\n` with `db.host=x` — error text contains `db` and `mapping`
- vars `a=x`, `a.b=y` (second passes through the string set by the first)

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoadVars`
Expected: build failure, `undefined: LoadVars`.

- [ ] **Step 3: Implement `LoadVars` in `internal/config/vars.go`**

Decode the file with `yaml.Unmarshal` into an `any` and switch on the result: `nil` is an empty map, `map[string]any` is used as is, anything else is the "must be a mapping" error. Split each `--var` on the first `=` (`strings.Cut`), split the key on `.`, walk and create intermediate `map[string]any` values, and fail when an intermediate value exists and is not a `map[string]any`. The final segment is assigned unconditionally.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add internal/config/vars.go internal/config/vars_test.go
git commit -m "feat(config): load template variables from a file and --var flags"
```

---

### Task 2: `template` command

**Files:**
- Create: `cmd/template.go`
- Modify: `cmd/root.go` (register the command next to `NewGenerateCommand()`)
- Test: `cmd/template_test.go`

**Interfaces:**
- Consumes: `config.LoadVars(varFile string, vars []string) (map[string]any, error)`, `config.ValuesKey`, `template.EvalGoTemplates(content string, data any) (string, error)`, `redact.Error(err error) error`.
- Produces: `func NewTemplateCommand() *cobra.Command` in package `cmd`.

Command shape: `Use: "template <template>"`, `SilenceUsage: true`, flags `--var` (`StringArrayVar`, repeatable) and `--var-file` (`StringArrayVar`, so a repeat is detectable; more than one value is an error). Output is written with `fmt.Fprint(cmd.OutOrStdout(), result)`.

- [ ] **Step 1: Write the failing tests** in `cmd/template_test.go` (package `cmd`).

Helper used by every test:

```go
// runTemplate executes the root command the way main does and returns what
// it wrote to stdout.
func runTemplate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand("2.0.3")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"template"}, args...))
	err := root.Execute()
	return out.String(), err
}
```

`TestTemplate_Output`, table-driven; stdout must equal `want` byte for byte, error must be nil. `<yaml>` is a temp file `v.yaml` holding `ns: from-file\nenv: prod\ndb:\n  host: a\n  port: 5432\n`; `<json>` is a temp file `v.json` holding `{"n": 1000000}`.

| name | args | want |
|---|---|---|
| function | `{{ krmgenGenerated }}` | `krmgen-2.0.3` |
| no newline added | `x` | `x` |
| trailing newline kept | `"x\n"` | `"x\n"` |
| var | `--var`, `ns=black widow`, `{{ .Values.ns }}` | `black widow` |
| var file | `--var-file`, `<yaml>`, `{{ .Values.ns }}/{{ .Values.env }}` | `from-file/prod` |
| var overrides file | `--var-file`, `<yaml>`, `--var`, `ns=black widow`, `{{ .Values.ns }}/{{ .Values.env }}` | `black widow/prod` |
| dotted var keeps siblings | `--var-file`, `<yaml>`, `--var`, `db.host=b`, `{{ .Values.db.host }}:{{ .Values.db.port }}` | `b:5432` |
| json number | `--var-file`, `<json>`, `{{ .Values.n }}` | `1000000` |
| comma in value | `--var`, `hosts=a,b`, `{{ .Values.hosts }}` | `a,b` |
| sprig | `--var`, `ns=abc`, `{{ upper .Values.ns }}` | `ABC` |
| leading dash after `--` | `--`, `-x` | `-x` |
| empty template | `` (one empty argument) | `` |

`TestTemplate_Errors`, each must return a non-nil error and write nothing to stdout:
- no arguments
- two positional arguments: `a`, `b`
- `{{ .Values.missing }}` — error text contains `missing` and `--var`
- `{{ .Values.ns` (parse error)
- `--var`, `ns`, `x` — error text contains `ns`
- `--var-file`, `<path to a .txt file>`, `x`
- `--var-file`, `<yaml>`, `--var-file`, `<yaml>`, `x` — error text contains `--var-file`

`TestTemplate_GenerateCommandUnchanged`: `NewRootCommand("x")` has a `generate` command whose flags do not include `var` or `var-file`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/ -run TestTemplate`
Expected: FAIL, `unknown command "template" for "krmgen"`.

- [ ] **Step 3: Implement `NewTemplateCommand` in `cmd/template.go` and register it in `cmd/root.go`**

Argument check follows `NewGenerateCommand`: an `Args` function returning an error unless exactly one positional argument is given. `RunE` returns `redact.Error(...)` of whatever fails. When evaluation fails with a message containing `map has no entry for key`, append a hint that names `--var` and `--var-file`; do not reuse `missingValueHint` from `cmd/generate.go`, which talks about KrmGen files.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./cmd/`
Expected: `ok`.

- [ ] **Step 5: Run the binary**

Run: `go run . template '{{ krmgenGenerated }}'; echo "[exit=$?]"`
Expected: `krmgen-[exit=0]` (a dev build has an empty version; no newline before `[`).

Run: `go run . template --var ns='black widow' --var env=prod '{{ .Values.ns }}/{{ .Values.env }}'`
Expected: `black widow/prod`

Run: `go run . template '{{ .Values.x }}'; echo "[exit=$?]"`
Expected: an `Error:` line on stderr naming `x` and `--var`, then `[exit=1]`.

- [ ] **Step 6: Commit**

```bash
git add cmd/template.go cmd/template_test.go cmd/root.go
git commit -m "feat: add template command to evaluate a Go template string"
```

---

### Task 3: Documentation

**Files:**
- Modify: `docs/specification.md` (section 1: Commands, Flags, Streams, Exit codes; new subsection for `template` variables)
- Modify: `README.md` (table of contents, new section for the command, project structure)
- Modify: `CLAUDE.md` (architecture tree, short section on the command)
- Modify: `docs/superpowers/specs/2026-11-19-template-command-design.md` (status line)

**Interfaces:**
- Consumes: the behaviour built in Tasks 1 and 2. Document what the code does; where it differs from the spec, the code wins and the difference is reported.
- Produces: nothing for other tasks.

- [ ] **Step 1: Update `docs/specification.md`**

Commands table gains `krmgen template <template>` (no alias). Flags table is split or extended so each flag names its command: `--skip` (`generate`), `--var` (`template`, string array, repeatable), `--var-file` (`template`, single). Streams: for a successful `template` run stdout holds the evaluated text exactly, with no newline added; the "stdout is valid YAML" guarantee stays scoped to `generate`. Exit codes: 0 and 1 cover `template` as well. New subsection states every rule from the spec's Decisions and Behaviour details as current behaviour, including: precedence, string-only `--var`, dotted keys and the non-mapping error, literal values, missing key is an error, one `--var-file`, `--` before a template that starts with `-`, and that function output and `--var` values are not hidden (stdout, shell history, process table).

- [ ] **Step 2: Update `README.md` and `CLAUDE.md`**

README: a section with the three examples from the spec's Goal and the two flags, plus a table-of-contents entry and the two new files in Project structure. CLAUDE.md: `cmd/template.go` and `internal/config/vars.go` in the architecture tree and a short section with one example.

- [ ] **Step 3: Set the design status**

In the design doc change the status line to `Status: implemented`.

- [ ] **Step 4: Verify**

Run: `go test ./...`
Expected: every package `ok` (this includes `dockerhub_doc_test.go`).

Run: `grep -c '{{' docs/dockerhub.md`
Expected: `0`

- [ ] **Step 5: Commit**

```bash
git add docs/specification.md README.md CLAUDE.md docs/superpowers/specs/2026-11-19-template-command-design.md
git commit -m "docs: document the template command"
```

---

## Final verification

Run: `go build ./... && go vet ./... && go test -race ./...`
Expected: every package `ok`.

Run: `golangci-lint run ./...` (v2.15.0 built with go1.27)
Expected: `0 issues.`
