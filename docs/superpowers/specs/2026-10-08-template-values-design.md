# Template values — design

Date: 2026-10-08
Status: approved design, not yet implemented

## Goal

Let a `krmgen.yaml` declare named values once — themselves computed by
templates — and use them from every templated file in the source tree:

```yaml
# krmgen.yaml
kind: KrmGen
values:
  clusterProfile: '{{ argocdEnv "CLUSTER_PROFILE" }}'
  keyvault: '{{ printf "rixocz-%s-aks-vault" .Values.clusterProfile }}'
```

```yaml
# secret.yaml (any file in the tree)
db-secret: {{ azSec .Values.keyvault "db-secret" }}
```

Success: the example above renders; a typo in a value name fails the run
instead of rendering garbage; existing configs without `values` render
byte-identically.

## Decisions

| Topic | Decision |
|---|---|
| Access syntax | Helm style: `.Values.<key>`; `$.Values.<key>` inside `range` / `with` |
| Evaluation | Values are read from the **raw** YAML and evaluated leaf by leaf, in document order; each leaf sees the values resolved before it |
| Quoting | A templated value must be a quoted YAML string — the file has to be valid YAML before templating |
| Multiple config files | `values` of all top-level `kind: KrmGen` files are merged; the same top-level key in two files is an error |
| Missing key | Error (`missingkey=error`), never `<no value>` |

Rejected: bare `$keyvault` (needs a preamble injected into every file — shifts
error line numbers, collides with user variables); evaluating the whole
`krmgen.yaml` first and reading `values` from the result (the file itself could
not use `.Values`); a separate values file kind (one more concept, no gain).

## Pipeline

`cmd/generate.go`, new step between reading skip patterns and copying:

1. `config.ReadSkipPatterns(srcDir)` — unchanged.
2. **`config.ResolveValues(srcDir)`** → `map[string]any`.
3. `copyDir` evaluates every non-skipped file with data
   `map[string]any{"Values": values}` — including `krmgen.yaml` itself and
   files in subdirectories and `kustomization.yaml`.
4. `processWorkDir` — unchanged.

The `values:` block of `krmgen.yaml` is evaluated a second time in step 3, now
with all values in scope; it yields the same strings. Azure lookups are not
repeated — the provider caches per process.

## `config.ResolveValues`

New file `internal/config/values.go`.

- Lists top-level regular files of `srcDir`, sorted by name (`os.ReadDir`
  order), and keeps those whose raw YAML parses and has `kind: KrmGen`.
  A file that does not parse is skipped, exactly like `ReadSkipPatterns`.
- Decodes the document into a `yaml.Node` and walks the `values` mapping in
  document order (a `map[string]any` would lose the order chaining depends on).
- Leaf rules:
  - `!!str` scalar → evaluated with `template.EvalGoTemplates(leaf, data)`
    where `data` is `{"Values": <values resolved so far>}`; the result is
    always a `string` (no YAML re-parse: `'{{ 3 }}'` is `"3"`).
  - other scalars (int, float, bool, null) → decoded and kept typed.
  - mappings and sequences → walked recursively, same rules.
- A value may reference only values resolved **earlier** (earlier files by
  name, then earlier keys in document order). A forward reference fails with
  the `missingkey` error.
- Top-level key present in two files → error naming both files.
- `values` present but not a mapping → error.
- No `values` anywhere → empty map; behaviour identical to today.

## Template engine

`template.EvalGoTemplates(content string, data any) (string, error)`:

- passes `data` to `tmpl.Execute`;
- sets `Option("missingkey=error")`. Existing templates use no data, so this
  changes nothing for them.

All callers updated (`cmd/generate.go`, `cmd/only-test`, tests).

## Types and schema

- `types.Config` gains `Values map[string]any \`yaml:"values"\`` —
  informational; runtime reads values through `ResolveValues`.
- `resources/krmgen-config-schema.json`: `values` as an object with arbitrary
  properties.
- `skip` stays untemplated, as today.

## Errors

All fail the run with exit code 1.

| Case | Message shape |
|---|---|
| Template error in a value | `evaluating value values.keyvault in krmgen.yaml failed error: ...` |
| Duplicate key | `value "keyvault" defined in both a.yaml and krmgen.yaml` |
| `values` not a mapping | `values in krmgen.yaml must be a mapping` |
| Missing key in a file | existing `template evaluation of file %s failed error: ... map has no entry for key "x"`, plus hint `(values are read from krmgen.yaml before templating — quote templated values)` when the missing key is under `.Values` |

A value error names the key path, never the evaluated result (values may hold
secrets). Errors still pass through `redact.Error`.

## Security

Values live in memory and in rendered files of the working directory — the
same places rendered templates already end up. Nothing new is written.
Files matching `skip` are not evaluated and get no values.

## Testing

TDD.

- `internal/config/values_test.go`: ordering and chaining; forward reference
  fails; nested maps and lists; typed non-string leaves; duplicate key across
  files; unparseable file skipped; `values` not a mapping; no `values` →
  empty map.
- `internal/template/template_test.go`: data reaches the template;
  `missingkey=error`; templates without `.Values` unchanged.
- `cmd` integration, fixture `test/resources/values/`: the Goal example with
  `argocdEnv` via `t.Setenv`; `.Values` in a subdirectory file and in
  `kustomization.yaml`; `$.Values` inside `range`; skipped file stays literal.
- Golden tests unchanged.

## Docs

- `docs/specification.md`: `values` row in Fields; new "Values" section;
  pipeline step for `ResolveValues`.
- `README.md`: example.
- `CLAUDE.md`: pipeline and architecture lines.
