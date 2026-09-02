# krmgen — Claude Code Project Guide

## Project overview

`krmgen` is a CLI tool for generating Kubernetes Resource Model (KRM) YAML from Helm charts and Kustomize configs. It is written in Go (module `github.com/librucha/krmgen`).

The core idea: take a `krmgen.yaml` config + optional `kustomization.yaml`, render every declared helm chart (through the embedded `helm.sh/helm/v4` library by default, or by the `helm` binary when `KRMGEN_HELM_EXECUTABLE` is set), optionally pipe the result through kustomize (rendered by the embedded library by default, or by `kubectl kustomize` when `KRMGEN_KUBECTL_EXECUTABLE` is set), and print the final YAML to stdout.

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
  types.go              → Config, Metadata, Helm, HelmChart types
  config/parser.go      → IsConfigFile, ParseConfig (YAML unmarshal)
  config/processor.go   → ProcessConfig: orchestrates helm + kustomize
  helm/
    generator.go        → generator interface, OCI vs HTTP repo detection
    repo-generator.go   → HTTP repo helm generator
    oci-generator.go    → OCI registry helm generator
    processor.go        → TemplateHelmCharts orchestrates charts; shared values/credentials helpers
    renderer.go          → Renderer interface, selectRenderer (embedded vs binary, keyed on KRMGEN_HELM_EXECUTABLE)
    renderer_sdk.go       → embedded backend, helm.sh/helm/v4/pkg/action (default)
    renderer_binary.go    → external backend, shells out to the helm binary
  kustomize/
    processor.go        → FindKustomizeFile, BuildKustomize (prepares the kustomization, then delegates to a Builder)
    builder.go          → Builder interface, selectBuilder (embedded vs kubectl, keyed on KRMGEN_KUBECTL_EXECUTABLE)
    builder_krusty.go   → embedded backend, sigs.k8s.io/kustomize/api (default)
    builder_kubectl.go  → external backend, shells out to kubectl kustomize
  template/
    template.go         → EvalGoTemplates — registers all template funcs
    argocd/             → argocdEnv func (ARGOCD_ENV_* / ARGOCD_APP_* env vars)
    kube/               → kubeEnv func (KUBE_* env vars)
    files/              → readF func (read local relative file)
    krmgen/             → krmgenVer, krmgenGenerated funcs
    (Azure functions — azSec, toPem, azPfxKey, azPfxCrt, azCert, azKey,
     azStoreKey, azUserIdentityClientId — come from the
     github.com/librucha/cloud-go-templates dependency, not from this repo)
  tool/tool.go          → RunCommand wrapper for external binaries
  utils/constants.go    → env var name constants
  utils/perm.go         → FilePerm / DirPerm — modes for rendered working files and directories

version/version.go      → AppVersion global var set at build time

docs/specification.md   → product contract: what krmgen accepts, produces, guarantees
```

## Template functions available in krmgen.yaml / kustomization.yaml

| Function | Description |
|---|---|
| `krmgenVer` | Current krmgen version |
| `krmgenGenerated` | "krmgen-<version>" label value |
| `azSec <vault> <secret> [version]` | Azure Key Vault secret |
| `toPem <type> <data>` | Wrap bytes in PEM block |
| `azPfxKey <vault> <secret> [version]` | Extract private key from PKCS12 secret |
| `azPfxCrt <vault> <secret> [version]` | Extract certificate(s) from PKCS12 secret |
| `azCert <vault> <cert> [version]` | Azure Key Vault certificate (PEM) |
| `azKey <vault> <key> [version]` | RSA modulus, PEM-encoded under a `"... PRIVATE KEY"` header — not actually a private key, see specification |
| `azStoreKey <subscription> <resourceGroup> <account>` | Azure Storage account key |
| `azUserIdentityClientId <resourceGroup> <name>` | Azure Managed Identity client ID |
| `azUaIdClientId <resourceGroup> <name>` | Deprecated alias for `azUserIdentityClientId`, kept for backward compatibility |
| `argocdEnv <key> [default]` | Read `ARGOCD_ENV_<key>` / `ARGOCD_APP_<key>` |
| `kubeEnv <key> [default]` | Read `KUBE_<key>` env var |
| `readF <relpath> [default]` | Read local file relative to krmgen's process working directory (not the source dir) |
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
| `KRMGEN_HELM_EXECUTABLE` | Opt into the external helm backend: that path is used as helm. Unset (the default) renders through the helm library compiled into krmgen instead (see specification) |
| `KRMGEN_HELM_USERNAME` | Helm repo username (fallback if not in config) |
| `KRMGEN_HELM_PASSWORD` | Helm repo password (fallback if not in config) |
| `KRMGEN_KUBECTL_EXECUTABLE` | Opt into the external kubectl backend for kustomize: that path is used as kubectl. Unset (the default) renders through the kustomize library compiled into krmgen instead (see specification) |

## Build & development

Uses [Task](https://taskfile.dev) as task runner. Common commands:

```bash
task build          # build binary to build/krmgen
task test           # run tests with race detector and coverage
task lint           # golangci-lint
task check          # fmt + vet + lint + test
task install        # go install to ~/go/bin
task install-release # download the latest published GitHub release into ~/bin
                    # (no build; override the target with INSTALL_DIR=)
task docker-build   # goreleaser snapshot (no publish)
task release        # goreleaser release + Docker push (needs DOCKER_USERNAME/PASSWORD)
```

Neither `helm` nor `kubectl` is required to build or run krmgen — both render through the libraries compiled in by default. `task test` needs both on PATH regardless: the golden harness packages fixture charts with `helm package`/`helm repo index`, and the differential parity tests exercise the external `KRMGEN_HELM_EXECUTABLE`/`KRMGEN_KUBECTL_EXECUTABLE` backends against the embedded ones.

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
- `resources/krmgen-config-schema.json` exists and is exercised by
  `internal/config/schema_test.go`, but `ParseConfig` (`internal/config/parser.go`)
  does not load or apply it at runtime — no validation schema is wired in
- Azure functions come from the `github.com/librucha/cloud-go-templates`
  library, not from this repo. The library's provider caches clients and
  results in memory, keyed on the call's own arguments (never on the resource
  ID Azure returns), for the lifetime of the provider; it is safe for
  concurrent use. See
  [`docs/specification.md`](docs/specification.md#4-template-functions),
  Caching, for details
- Production code returns errors up the call stack instead of calling `log.Fatal`; cobra prints the
  returned error to stderr (`Error: ...`) and `main` (`krmgen.go`) exits with code 1. Informational
  `log.Println` calls (e.g. `internal/config/parser.go` on an unreadable file) still stay — only
  fatal exits were replaced. `cmd/only-test/run.go` is a development utility outside the shipped
  binary (`.goreleaser.yaml` builds `main: .`) and keeps its two `log.Fatal` calls.

## Specification

[`docs/specification.md`](docs/specification.md) is the product contract —
what krmgen accepts, produces, and guarantees. It documents current behaviour
(including known bugs and deviations) rather than intended behaviour, and is
the reference later refactoring phases are measured against.
