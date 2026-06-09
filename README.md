# krmgen

[![Build](https://github.com/librucha/krmgen/actions/workflows/go.yml/badge.svg)](https://github.com/librucha/krmgen/actions/workflows/go.yml)
[![Release](https://img.shields.io/github/v/release/librucha/krmgen)](https://github.com/librucha/krmgen/releases/latest)
[![Docker](https://img.shields.io/docker/v/librucha/krmgen?label=docker)](https://hub.docker.com/r/librucha/krmgen)
[![Go Report Card](https://goreportcard.com/badge/github.com/librucha/krmgen)](https://goreportcard.com/report/github.com/librucha/krmgen)

**krmgen** is a CLI tool that generates Kubernetes resource manifests by combining Helm chart templating with Kustomize overlays — and evaluating Go templates in config files before either tool runs. This lets you inject secrets, environment-specific values, and dynamic data directly into your manifests at generation time, without any cluster-side tooling.

Designed for GitOps pipelines (ArgoCD, Flux) where you need fully-rendered, static YAML as output.

---

## Table of contents

- [How it works](#how-it-works)
- [Features](#features)
- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Configuration reference](#configuration-reference)
- [Template functions](#template-functions)
- [Examples](#examples)
- [Docker](#docker)
- [Environment variables](#environment-variables)
- [Development](#development)

---

## How it works

```
krmgen generate <path>
         │
         ▼
  1. Copy source directory to temp dir
     Evaluate Go templates in every file
     (files matching skip patterns are copied as-is)
         │
         ▼
  2. Find krmgen.yaml  (kind: KrmGen)
     Run `helm template` for each declared chart
         │
         ▼
  3. If kustomization.yaml exists:
     Inject Helm output as a resource and run `kubectl kustomize`
         │
         ▼
  4. Print final YAML to stdout
```

The key insight is step 1: **all files are Go-template-evaluated before Helm or Kustomize sees them**. This means you can use template functions anywhere — in `krmgen.yaml` values, `kustomization.yaml`, or Kubernetes manifests under `kustomize/`.

---

## Features

- **Helm + Kustomize in one pass** — renders Helm charts and pipes the result through Kustomize automatically
- **Go template engine** — full [sprig](https://masterminds.github.io/sprig/) function library available in all config files
- **Azure Key Vault integration** — fetch secrets, certificates, and keys directly into templates
- **ArgoCD & Kubernetes env vars** — read `ARGOCD_ENV_*` / `ARGOCD_APP_*` / `KUBE_*` variables
- **Local file inclusion** — embed file contents into templates with `readF`
- **Skip patterns** — exclude binary or generated files from template evaluation via glob patterns (`*.pfx`, `assets/*.png`)
- **OCI registry support** — Helm charts from OCI registries (`oci://`)
- **Docker image** — `librucha/krmgen` available for CI pipelines

---

## Prerequisites

- [`helm`](https://helm.sh/docs/intro/install/) — must be in `PATH`
- [`kubectl`](https://kubernetes.io/docs/tasks/tools/) — must be in `PATH` (only required when `kustomization.yaml` is present)

---

## Installation

### Binary (recommended)

Download the latest binary for your platform from [GitHub Releases](https://github.com/librucha/krmgen/releases/latest).

```bash
# Linux amd64
curl -L https://github.com/librucha/krmgen/releases/latest/download/krmgen_linux_x86_64.tar.gz | tar xz && sudo mv krmgen /usr/local/bin/

# Linux arm64
curl -L https://github.com/librucha/krmgen/releases/latest/download/krmgen_linux_arm64.tar.gz | tar xz && sudo mv krmgen /usr/local/bin/
```

### Docker

```bash
docker pull librucha/krmgen:latest
```

### Go install

```bash
go install github.com/librucha/krmgen@latest
```

---

## Quick start

**1. Create a `krmgen.yaml` config:**

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen

helm:
  charts:
    - name: hello-world
      repo: https://helm.github.io/examples
      releaseName: my-app
      version: 0.1.0
      namespace: default
```

**2. Run the generator:**

```bash
krmgen generate .
```

**3. Capture the output:**

```bash
krmgen generate . > manifests.yaml
```

---

## Configuration reference

### `krmgen.yaml`

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen

metadata:                          # optional, not propagated to output resources
  labels:
    app: my-app
  annotations:
    note: some-note

skip:                              # optional — glob patterns of files to copy without template evaluation
  - "*.pfx"                        #   matches any .pfx file regardless of directory depth
  - "*.png"
  - "certs/*.pem"                  #   directory-scoped: only .pem files inside certs/

helm:
  charts:
    - name: <chart-name>           # required — Helm chart name
      repo: <repo-url>             # required — HTTP(S) or oci:// repo URL
      releaseName: <release>       # required — Helm release name
      version: <version>           # optional — chart version (latest if omitted)
      namespace: <namespace>       # optional — target namespace
      ignoreCredentials: false     # optional — skip auth for public OCI repos
      repoUser: <username>         # optional — repo username
      repoPassword: <password>     # optional — repo password (use env var or template)
      valuesFile: values.yaml      # optional — path to values file (relative to config)
      valuesInline:                # optional — inline Helm values
        key: value
```

> Any string value in this file can use Go template syntax, e.g. `'{{ argocdEnv "MY_VAR" }}'`

The `--skip` flag can also be passed on the command line and is merged with patterns from `krmgen.yaml`:

```bash
krmgen generate . --skip='*.pfx' --skip='assets/*.png'
```

Patterns use [`filepath.Match`](https://pkg.go.dev/path/filepath#Match) syntax. A pattern is tested against both the full relative path and the bare filename, so `*.pfx` matches `certs/prod/cert.pfx` without needing a directory prefix.

### JSON Schema

A JSON schema for IDE autocompletion is available at:

```
https://raw.githubusercontent.com/librucha/krmgen/master/krmgen-config-schema.json
```

For JetBrains IDEs, add it in **Settings → Languages & Frameworks → Schemas and DTDs → JSON Schema Mappings**.

---

## Template functions

All files in the source directory are evaluated as Go templates before processing. The full [sprig](https://masterminds.github.io/sprig/) function library is available, except `env` and `expandenv` (disabled for security).

### krmgen

| Function | Description | Example |
|---|---|---|
| `krmgenVer` | Current krmgen version | `{{ krmgenVer }}` |
| `krmgenGenerated` | Value for `generated-by` labels | `{{ krmgenGenerated }}` |

### Environment

| Function | Signature | Description |
|---|---|---|
| `argocdEnv` | `argocdEnv <key> [default]` | Read `ARGOCD_ENV_<key>` or `ARGOCD_APP_<key>` env var |
| `kubeEnv` | `kubeEnv <key> [default]` | Read `KUBE_<key>` env var |

```yaml
namespace: '{{ argocdEnv "TARGET_NAMESPACE" "default" }}'
replicas: '{{ kubeEnv "REPLICA_COUNT" "2" }}'
```

### Files

| Function | Signature | Description |
|---|---|---|
| `readF` | `readF <relpath> [default]` | Read a local file relative to the source directory |

```yaml
someConfig: '{{ readF "config/app.conf" "" }}'
```

### Azure Key Vault — Secrets

Authenticates via [`DefaultAzureCredential`](https://learn.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication) (Workload Identity, Managed Identity, env vars, Azure CLI, etc.).

| Function | Signature | Description |
|---|---|---|
| `azSec` | `azSec <vault> <secret> [version]` | Get secret value. Without version: returns latest version that is enabled and has no future `NotBefore` date |
| `toPem` | `toPem <blockType> <data>` | Wrap raw bytes in a PEM block |
| `azPfxKey` | `azPfxKey <vault> <secret> [version]` | Extract private key (PKCS#8 PEM) from a PKCS#12 secret |
| `azPfxCrt` | `azPfxCrt <vault> <secret> [version]` | Extract certificate(s) (PEM) from a PKCS#12 secret |

```yaml
# Secret value
password: '{{ azSec "my-vault" "db-password" }}'

# Specific version
password: '{{ azSec "my-vault" "db-password" "abc123def456" }}'

# PKCS#12 — split into key + cert
tlsKey: '{{ azPfxKey "my-vault" "tls-cert" }}'
tlsCrt: '{{ azPfxCrt "my-vault" "tls-cert" }}'
```

### Azure Key Vault — Certificates

| Function | Signature | Description |
|---|---|---|
| `azCert` | `azCert <vault> <cert> [version]` | Get certificate in PEM format |

```yaml
caCert: '{{ azCert "my-vault" "root-ca" }}'
```

### Azure Key Vault — Keys

| Function | Signature | Description |
|---|---|---|
| `azKey` | `azKey <vault> <key> [version]` | Get public key in PEM format |

```yaml
publicKey: '{{ azKey "my-vault" "signing-key" }}'
```

### Azure Storage

| Function | Signature | Description |
|---|---|---|
| `azStoreKey` | `azStoreKey <subscriptionId> <resourceGroup> <accountName>` | Get storage account primary key |

```yaml
storageKey: '{{ azStoreKey "00000000-0000-0000-0000-000000000000" "my-rg" "mystorageaccount" }}'
```

### Azure Managed Identity

| Function | Signature | Description |
|---|---|---|
| `azUaIdClientId` | `azUaIdClientId <resourceGroup> <identityName>` | Get user-assigned managed identity client ID |

```yaml
clientId: '{{ azUaIdClientId "my-rg" "my-workload-identity" }}'
```

---

## Examples

### Helm only

```yaml
# krmgen.yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen

helm:
  charts:
    - name: cert-manager
      repo: https://charts.jetstack.io
      releaseName: cert-manager
      namespace: cert-manager
      version: v1.14.0
      valuesInline:
        installCRDs: true
```

```bash
krmgen generate . > cert-manager.yaml
```

### Helm + Kustomize

```yaml
# krmgen.yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen

helm:
  charts:
    - name: my-app
      repo: oci://registry.example.com/charts
      releaseName: my-app
      namespace: production
      version: 1.2.3
```

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: production

labels:
  - pairs:
      app.kubernetes.io/generated-by: '{{ krmgenGenerated }}'

# Helm output is automatically injected here by krmgen
```

### ArgoCD pipeline with dynamic values

```yaml
# krmgen.yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen

helm:
  charts:
    - name: my-app
      repo: oci://registry.example.com/charts
      releaseName: '{{ argocdEnv "APP_NAME" }}'
      namespace: '{{ argocdEnv "TARGET_NAMESPACE" "default" }}'
      version: '{{ argocdEnv "APP_VERSION" }}'
      valuesInline:
        image:
          tag: '{{ argocdEnv "IMAGE_TAG" "latest" }}'
        replicaCount: '{{ argocdEnv "REPLICAS" "2" }}'
```

### Azure secrets in Kubernetes Secret

```yaml
# kustomize/resources/app-secrets.yaml
apiVersion: v1
kind: Secret
metadata:
  name: app-secrets
type: Opaque
stringData:
  db-password: '{{ azSec "my-vault" "prod-db-password" }}'
  tls.key: '{{ azPfxKey "my-vault" "tls-cert" }}'
  tls.crt: '{{ azPfxCrt "my-vault" "tls-cert" }}'
  storage-key: '{{ azStoreKey "sub-id" "my-rg" "mystorageaccount" }}'
```

### Skipping binary files

When a source directory contains binary files (certificates, images, archives), template evaluation would fail on them. Use `skip` to copy them unchanged:

```yaml
# krmgen.yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen

skip:
  - "*.pfx"        # PKCS#12 bundles
  - "*.p12"
  - "*.png"
  - "*.jpg"

helm:
  charts:
    - name: my-app
      repo: https://charts.example.com
      releaseName: my-app
      version: 1.0.0
```

Alternatively, pass patterns at call time without modifying `krmgen.yaml`:

```bash
krmgen generate . --skip='*.pfx' --skip='*.p12'
```

Both sources are merged and deduplicated, so you can combine project-level defaults in `krmgen.yaml` with ad-hoc overrides on the CLI.

### OCI registry with credentials

```yaml
# krmgen.yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen

helm:
  charts:
    - name: my-private-app
      repo: oci://private.registry.example.com/charts
      releaseName: my-private-app
      repoUser: '{{ argocdEnv "REGISTRY_USER" }}'
      repoPassword: '{{ argocdEnv "REGISTRY_PASSWORD" }}'
      version: 2.0.0

    - name: public-app
      repo: oci://quay.io/my-org/charts
      releaseName: public-app
      ignoreCredentials: true    # skip auth for public registries
      version: 1.0.0
```

---

## Docker

```bash
# Print version
docker run librucha/krmgen:latest krmgen --version

# Generate manifests from local directory
docker run --rm -v "$(pwd):/workspace" librucha/krmgen:latest krmgen generate /workspace

# With Azure credentials via environment
docker run --rm \
  -v "$(pwd):/workspace" \
  -e AZURE_TENANT_ID \
  -e AZURE_CLIENT_ID \
  -e AZURE_CLIENT_SECRET \
  librucha/krmgen:latest krmgen generate /workspace

# Capture output
docker run --rm -v "$(pwd):/workspace" librucha/krmgen:latest krmgen generate /workspace > manifests.yaml
```

---

## Environment variables

| Variable | Description |
|---|---|
| `KRMGEN_HELM_EXECUTABLE` | Override path to `helm` binary |
| `KRMGEN_HELM_USERNAME` | Helm repo username (fallback if not set in `krmgen.yaml`) |
| `KRMGEN_HELM_PASSWORD` | Helm repo password (fallback if not set in `krmgen.yaml`) |
| `KRMGEN_KUBECTL_EXECUTABLE` | Override path to `kubectl` binary |

For Azure authentication, krmgen uses the standard Azure SDK environment variables:

| Variable | Description |
|---|---|
| `AZURE_TENANT_ID` | Azure tenant ID |
| `AZURE_CLIENT_ID` | Service principal / managed identity client ID |
| `AZURE_CLIENT_SECRET` | Service principal client secret |

See [Azure SDK authentication](https://learn.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication) for all supported authentication methods.

---

## Development

### Requirements

- Go 1.21+
- [Task](https://taskfile.dev) (`brew install go-task`)
- `helm` and `kubectl` in PATH

### Common tasks

```bash
task build          # compile binary to build/krmgen
task test           # run tests with race detector and coverage
task lint           # run golangci-lint
task check          # fmt + vet + lint + test
task install        # install to ~/go/bin
task docker-build   # build Docker image locally (goreleaser snapshot)
```

### Adding a new template function

1. Create a provider package under `internal/template/<name>/`
2. Export a `const FuncName = "myFuncName"` and the implementation function
3. Register it in `internal/template/template.go` inside `initFuncs()`
4. Add tests in the provider package

### Project structure

```
cmd/                    CLI commands (cobra)
internal/
  config/               krmgen.yaml parsing and processing
  helm/                 helm template execution (HTTP + OCI generators)
  kustomize/            kubectl kustomize execution
  template/             Go template engine + all function providers
    azure/              Azure Key Vault, storage, identity providers
    argocd/             ArgoCD env var provider
    kube/               Kubernetes env var provider
    files/              Local file reader
  utils/                Shared constants
version/                Build-time version variable
```

---

## License

[MIT](LICENSE)
