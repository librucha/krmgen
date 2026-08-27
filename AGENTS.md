# krmgen — Agent Guide

## Project summary

CLI tool (`krmgen`) that generates Kubernetes YAML by running `helm template` + Kustomize (rendered by the embedded `sigs.k8s.io/kustomize/api` library by default, or by `kubectl kustomize` when `KRMGEN_KUBECTL_EXECUTABLE` is set) and evaluating Go templates in config files. Written in Go.

## Key entry points

- `krmgen.go` — main; wires build-time version
- `cmd/generate.go` — the generate command (copies dir, evaluates templates, processes config)
- `internal/config/processor.go` — orchestrates helm + kustomize
- `internal/template/template.go` — registers all template functions

## How to build and test

```bash
# build
task build

# run all tests
task test

# lint
task lint

# full check (fmt + vet + lint + test)
task check
```

If `task` is not installed: `go build -o build/krmgen .` and `go test ./...`

## Important constraints

- External binary `helm` must be present in PATH. `kubectl` is only needed if `KRMGEN_KUBECTL_EXECUTABLE` opts into the external Kustomize backend — the default path renders through the embedded library and needs no `kubectl`.
- Template functions `env` and `expandenv` are intentionally removed from sprig for security.
- Azure providers use `azidentity.NewDefaultAzureCredential` — requires valid Azure auth in environment.
- Production code returns errors up the call stack instead of calling `log.Fatal`; cobra prints the
  returned error to stderr (`Error: ...`) and `main` (`krmgen.go`) exits with code 1.
- `valuesInline` in HelmChart config generates a temp file per chart in workDir.

## Where things live

| Concern | Path |
|---|---|
| CLI commands | `cmd/` |
| Core types | `internal/types.go` |
| Config parsing | `internal/config/` |
| Helm execution | `internal/helm/` |
| Kustomize execution | `internal/kustomize/` |
| Template functions | `internal/template/` |
| Azure providers | `github.com/librucha/cloud-go-templates` (external dependency, not in this repo) |
| Env/file providers | `internal/template/argocd/`, `kube/`, `files/` |
| Constants | `internal/utils/constants.go` |
| Test fixtures | `test/resources/` |

## Adding a new template function

1. Create a new provider package under `internal/template/<name>/`
2. Export a `const FuncName = "myFunc"` and a function with matching signature
3. Register it in `internal/template/template.go` `initFuncs()`
4. Add a test in the provider package

## Adding a new Helm generator type

1. Implement the `generator` interface in `internal/helm/generator.go`
2. Add detection logic in `newGenerator()` based on URL prefix
