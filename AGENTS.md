# krmgen — Agent Guide

## Project summary

CLI tool (`krmgen`) that generates Kubernetes YAML by running `helm template` + `kubectl kustomize` and evaluating Go templates in config files. Written in Go.

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

- External binaries `helm` and `kubectl` must be present in PATH for integration to work.
- Template functions `env` and `expandenv` are intentionally removed from sprig for security.
- Azure providers use `azidentity.NewDefaultAzureCredential` — requires valid Azure auth in environment.
- Errors use `log.Fatal` (intentional CLI pattern — no error recovery).
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
| Azure providers | `internal/template/azure/` |
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
