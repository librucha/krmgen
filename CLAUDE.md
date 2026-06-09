# krmgen — Claude Code Project Guide

## Project overview

`krmgen` is a CLI tool for generating Kubernetes Resource Model (KRM) YAML from Helm charts and Kustomize configs. It is written in Go (module `github.com/librucha/krmgen`).

The core idea: take a `krmgen.yaml` config + optional `kustomization.yaml`, run `helm template` for every declared chart, optionally pipe the result through `kubectl kustomize`, and print the final YAML to stdout.

## Architecture

```
krmgen.go          → entry point, wires version into cmd
cmd/root.go        → cobra root command
cmd/generate.go    → "generate <path>" command:
                      1. read skip patterns from krmgen.yaml (pre-copy, raw YAML)
                      2. copy src dir to temp dir (evaluating Go templates in all files
                         except those matching skip patterns — copied as-is)
                      3. find KrmGen config files (kind: KrmGen)
                      4. ProcessConfig → helm + kustomize → stdout

internal/
  types.go              → Config, Helm, HelmChart, SecreteProvider types
  config/parser.go      → IsConfigFile, ParseConfig (YAML unmarshal)
  config/processor.go   → ProcessConfig: orchestrates helm + kustomize
  helm/
    generator.go        → generator interface, OCI vs HTTP repo detection
    repo-generator.go   → HTTP repo helm generator
    oci-generator.go    → OCI registry helm generator
    processor.go        → TemplateHelmCharts, runs `helm template` binary
  kustomize/
    processor.go        → FindKustomizeFile, BuildKustomize (kubectl kustomize)
  template/
    template.go         → EvalGoTemplates — registers all template funcs
    argocd/             → argocdEnv func (ARGOCD_ENV_* / ARGOCD_APP_* env vars)
    kube/               → kubeEnv func (KUBE_* env vars)
    files/              → readF func (read local relative file)
    krmgen/             → krmgenVer, krmgenGenerated funcs
    azure/
      sec/              → azSec, toPem, azPfxKey, azPfxCrt (Key Vault secrets + PKCS12)
      cert/             → azCert (Key Vault certificates)
      key/              → azKey (Key Vault keys)
      storage/          → azStoreKey (Storage account key)
      identity/         → azClientId (Managed Identity client ID)
      commons/          → shared subscription helpers
  tool/tool.go          → RunCommand wrapper for external binaries
  utils/constants.go    → env var name constants

version/version.go      → AppVersion global var set at build time
```

## Template functions available in krmgen.yaml / kustomization.yaml

| Function | Description |
|---|---|
| `krmgenVer` | Current krmgen version |
| `krmgenGenerated` | "krmgen-<version>" label value |
| `azSec <vault> <secret> [version]` | Azure Key Vault secret |
| `toPem <type> <data>` | Wrap bytes in PEM block |
| `azPfxKey <vault> <secret>` | Extract private key from PKCS12 secret |
| `azPfxCrt <vault> <secret>` | Extract certificate(s) from PKCS12 secret |
| `azCert <vault> <cert> [version]` | Azure Key Vault certificate (PEM) |
| `azKey <vault> <key> [version]` | Azure Key Vault key |
| `azStoreKey <account> <group>` | Azure Storage account key |
| `azClientId <sub> <group> <name>` | Azure Managed Identity client ID |
| `argocdEnv <key> [default]` | Read `ARGOCD_ENV_<key>` / `ARGOCD_APP_<key>` |
| `kubeEnv <key> [default]` | Read `KUBE_<key>` env var |
| `readF <relpath> [default]` | Read local file relative to source dir |
| All sprig functions | Except `env` and `expandenv` (security) |

## Skipping template evaluation

Files matching a glob pattern can be excluded from Go template evaluation (e.g. binary files, certificates).
They are still copied to the working directory unchanged.

**Via `krmgen.yaml`** (version-controlled, applied per project):
```yaml
kind: KrmGen
skip:
  - "*.pfx"
  - "*.png"
  - "certs/*.pem"
```

**Via CLI flag** (ad-hoc, overrides/extends config):
```bash
krmgen generate . --skip='*.pfx' --skip='assets/*.png'
```

Patterns use `filepath.Match` syntax. Each pattern is tested against both the full relative path
and the bare filename, so `*.pfx` matches `certs/prod/cert.pfx` without a directory prefix.

## Environment variables

| Variable | Description |
|---|---|
| `KRMGEN_HELM_EXECUTABLE` | Override helm binary path |
| `KRMGEN_HELM_USERNAME` | Helm repo username (fallback if not in config) |
| `KRMGEN_HELM_PASSWORD` | Helm repo password (fallback if not in config) |
| `KRMGEN_KUBECTL_EXECUTABLE` | Override kubectl binary path |

## Build & development

Uses [Task](https://taskfile.dev) as task runner. Common commands:

```bash
task build          # build binary to build/krmgen
task test           # run tests with race detector and coverage
task lint           # golangci-lint
task check          # fmt + vet + lint + test
task install        # go install to ~/go/bin
task docker-build   # goreleaser snapshot (no publish)
task release        # goreleaser release + Docker push (needs DOCKER_USERNAME/PASSWORD)
```

Required external tools: `helm`, `kubectl` (both must be in PATH or configured via env).

## Release process

- Managed by **goreleaser** (`.goreleaser.yaml`)
- Builds for `linux/amd64` and `linux/arm64`
- Docker image: `librucha/krmgen` on Docker Hub
- Version injected via `-X main.version={{.Version}}` ldflags

## Testing

- Unit tests alongside source files (`*_test.go`)
- Integration test resources under `test/resources/`
- Run: `task test`

## Code conventions

- All comments in English
- No validation schema wired (commented out in `parser.go`) — can be enabled
- Azure clients and secrets are cached in-memory per process run
- `log.Fatal` used throughout (process exits on any error — intentional for a CLI tool)
