# `krmgen template` command — design

Date: 2026-11-19
Status: implemented

## Goal

Evaluate one Go template string with krmgen's template functions and print
the result, without a source directory or a `krmgen.yaml`:

```bash
krmgen template '{{ krmgenGenerated }}'
# krmgen-2.0.3

krmgen template --var-file variables.yaml --var ns='black widow' --var env=prod \
  '{{ .Values.ns }}/{{ .Values.env }}'
# black widow/prod

krmgen template --var keyvault=rixocz-dev-aks-vault '{{ azSec .Values.keyvault "db-secret" }}'
```

Why was not stated by the requester. Assumed (author's reading): try a
template function or a snippet from a project file quickly, and use krmgen's
functions from shell scripts.

Success: the three examples above work; a snippet copied from a file rendered
by `krmgen generate` evaluates unchanged; `krmgen generate` behaves exactly as
before.

## Decisions

"Requester" marks what the requester decided. "Author" marks a call made by
the design author that the requester has seen and not objected to.

| Topic | Decision | By |
|---|---|---|
| Command | `krmgen template <template>`, exactly one positional argument | Requester |
| Variables file | `--var-file <file>`, extension `.yaml`, `.yml` or `.json` | Requester |
| Single variables | `--var key=value`, repeatable | Requester |
| Where variables appear | Under `.Values`, the same root key `generate` uses | Requester |
| Parser | `gopkg.in/yaml.v3` for all three extensions | Requester |
| Precedence | `--var` overrides the same key from `--var-file`; a later `--var` overrides an earlier one | Author |
| `--var` value type | Always a string. Numbers, lists and nested maps come from the file | Author |
| Dotted `--var` key | Sets a nested value: `--var db.host=x` gives `.Values.db.host` and replaces only that leaf of a `db` mapping loaded from the file | Author |
| Variable values | Literal. They are not evaluated as templates, unlike `values:` in `krmgen.yaml` | Author |
| Unknown key in the template | Error (`missingkey=error`), same as `generate` | Author |
| Output | The evaluated text exactly, to stdout, with no newline added | Author |
| Errors | Returned up to cobra, wrapped in `redact.Error`; exit code 1 | Author |
| `--var-file` count | One file. Passing the flag twice is an error rather than silently using the last one | Author |

## Non-goals

- Reading the template from stdin or from a file.
- Reading `values:` from a `krmgen.yaml`, or any link to a source directory.
- `--var` / `--var-file` on `krmgen generate`.
- Typed `--var` values (`--var replicas=3` as a number).
- Several `--var-file` flags merged together.
- A command alias (`generate` has `g`; none was asked for here).

## Anti-goals

- Any change in what `krmgen generate` renders.
- A second template engine or function set. The command must evaluate through
  `template.EvalGoTemplates`, so both commands always expose the same functions.

## Constraints

- `template.EvalGoTemplates(content, data)` (`internal/template/template.go`)
  already does the evaluation and stays untouched. Constructing the Azure
  provider needs no credentials, so `krmgen template '{{ krmgenGenerated }}'`
  works on a machine with no Azure login.
- `--var` must be registered with cobra's `StringArrayVar`, not
  `StringSliceVar`: the slice variant splits a value on commas, which would
  break `--var hosts=a,b`.
- `docs/dockerhub.md` must stay free of `{{` (guarded by
  `dockerhub_doc_test.go`), so template examples do not go there.
- `readF` resolves against the process working directory; nothing changes.
- Function output such as `azSec` reaches stdout in plain text, and `--var`
  values are visible in shell history and the process table. Same exposure as
  `generate`; the documentation says so.

## Approaches

1. **New command plus a small loader (chosen).** `cmd/template.go` holds the
   cobra command; `internal/config/vars.go` loads the file, applies the
   `--var` flags and returns `map[string]any`; the command calls
   `template.EvalGoTemplates(arg, map[string]any{config.ValuesKey: vars})`.
   The loader is testable without cobra.
2. **Flags on `krmgen generate`.** Rejected: `generate` takes a directory and
   emits KRM; evaluating a single string is a different job.
3. **Route variables through `config.ResolveValues`.** Rejected: it reads
   KrmGen files and evaluates each value as a template, top to bottom. These
   variables are literal, and there is no source directory.

For `.json`, `encoding/json` was considered and rejected: it decodes every
number as `float64`, so `{"n": 1000000}` would print `1e+06`. `yaml.v3` gives
the same types as `values:` in `krmgen.yaml`. Accepted cost: a `.json` file
containing YAML-only syntax is not rejected.

## Behaviour details

`--var-file`:
- Extension is matched case-insensitively. Any other extension is an error
  naming the three accepted ones.
- The top level must be a mapping. An empty file yields no variables.
- An unreadable or unparsable file is an error naming the file.

`--var key=value`:
- Split on the first `=`. The value may be empty (`--var x=`) and may contain
  `=`.
- No `=`, an empty key, or an empty path segment (`a..b`, `.a`) is an error.
- A dotted key that has to pass through an existing non-mapping value is an
  error. Example: the file has `db: prod-db` and the flag is
  `--var db.host=x`. Silently turning `db` into a mapping would make
  `{{ .Values.db }}` print `map[host:x]`.

Template argument:
- Zero or more than one positional argument is an error.

With no variables at all, `.Values` is an empty map, so `{{ .Values.x }}`
fails with the missing-key error rather than a nil-pointer one.

## Documentation to update

- `docs/specification.md`: section 1 (Commands, Flags, Streams, Exit codes)
  and a short section describing variables.
- `README.md`: usage section for the new command.
- `CLAUDE.md`: architecture tree (`cmd/template.go`,
  `internal/config/vars.go`) and a note on the command.

## Tests

- `internal/config/vars_test.go`: table tests for the file loader (three
  extensions, bad extension, non-mapping top level, empty file) and for
  `--var` (override, nested path, each error case, value containing `=`).
- `cmd/template_test.go`: the command end to end through cobra — the three
  examples from Goal (without Azure), exact output with no added newline,
  argument-count errors, missing key error.

## Left to the builder

- Wording of error messages and of the `--help` text.
- A hint appended to the missing-key error pointing at `--var` /
  `--var-file`. `missingValueHint` in `cmd/generate.go` talks about KrmGen
  files and does not fit here.
- Handling of a multi-document YAML file and of non-string mapping keys.
- Whether the loader lives in `internal/config` or its own package, if
  `internal/config` turns out to be the wrong home.

## Changes made during implementation

Found by the review of the finished branch; the code and
`docs/specification.md` are authoritative where they differ from the text
above.

- **`--var` errors do not quote the argument.** The value may be a secret and
  stderr ends up in logs. An error names the flag by position and, when it has
  one, the key: `invalid --var #2: key "a.password": a is already set and is
  not a mapping`.
- **A key in the variables file is its text, at any depth.** The file is
  walked as a `yaml.Node` tree instead of being unmarshalled generically,
  which turned `ports: {80: http}` into a `map[any]any` that templates could
  not read. As under `values:`, a repeated key and a YAML merge key are errors.
- **A null level is filled in by a dotted `--var`** (`db:` with nothing below,
  then `--var db.host=x`) instead of failing as "not a mapping".
- **An empty `--var-file` path is an error**, not "no file".
- **Flag-parsing errors are masked too**, and an unknown-flag error says that
  a template starting with a dash must follow `--`.
- **Known limit of "yaml.v3 for all three extensions":** valid JSON that uses
  the `\/` escape or a `\uXXXX` surrogate pair is rejected with a YAML parse
  error. Kept as decided and documented; the alternative is `encoding/json`
  with `UseNumber` for `.json`, which keeps integers intact as well.
