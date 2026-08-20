# krmgen specification

This document defines the contract of krmgen: what it accepts, what it produces,
and what guarantees hold. It is the reference against which implementation changes
are measured — if behaviour differs from this document, one of the two is a bug.

Version: applies to krmgen 0.x. Status: draft.

## 1. CLI contract

### Commands

| Command | Aliases | Description |
|---|---|---|
| `krmgen generate <path>` | `g` | Render KRM YAML from the config found in `<path>` |

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--skip` | string array, repeatable | none | Glob pattern of files copied without Go template evaluation |

### Streams

| Stream | Content |
|---|---|
| stdout | Rendered YAML, and nothing else |
| stderr | Log messages, warnings, errors |

Anything consuming krmgen may redirect stderr to `/dev/null` and still receive
valid YAML on stdout. This separation is a guarantee, not an implementation detail.

### Exit codes

| Code | Meaning |
|---|---|
| 0 | Rendering succeeded; YAML written to stdout |
| 1 | Any error: missing argument, unreadable path, template failure, helm or kustomize failure |

krmgen currently does not distinguish error classes by exit code. Callers must not
rely on a specific non-zero value beyond "non-zero means failure".

## 2. Configuration

krmgen looks for config files in the **top level** of the source directory (not
recursively). A file is a krmgen config when its parsed YAML has `kind: KrmGen`.
Every matching file is processed; each produces its own output block.

The machine-readable contract is [`resources/krmgen-config-schema.json`](../resources/krmgen-config-schema.json).

### Example

```yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen
skip:
  - "*.pfx"
helm:
  charts:
    - name: app
      repo: oci://registry.example.com/helm/app
      version: 3.0.0
      releaseName: app
      namespace: default
      valuesFile: values.yaml
      valuesInline:
        replicaCount: 2
```

### Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `apiVersion` | string | no | Informational; not validated against a known set |
| `kind` | string | **yes** | Must be `KrmGen` |
| `metadata.labels` | map[string]string | no | Currently parsed but not applied to output |
| `metadata.annotations` | map[string]string | no | Currently parsed but not applied to output |
| `skip` | []string | no | Glob patterns; see Rendering pipeline |
| `helm.charts[].name` | string | **yes** | Chart name |
| `helm.charts[].repo` | string | no | Repository URL; `oci://` selects the OCI backend, `http(s)://` the repo backend |
| `helm.charts[].version` | string | no | Chart version; omitted means latest |
| `helm.charts[].releaseName` | string | no | Helm release name |
| `helm.charts[].namespace` | string | no | Target namespace |
| `helm.charts[].repoUser` | string | no | Falls back to `KRMGEN_HELM_USERNAME` |
| `helm.charts[].repoPassword` | string | no | Falls back to `KRMGEN_HELM_PASSWORD` |
| `helm.charts[].ignoreCredentials` | bool | no | When true, no credentials are passed even if available |
| `helm.charts[].valuesFile` | string | no | Path relative to the source directory |
| `helm.charts[].valuesInline` | object | no | Written to a temporary values file |

`metadata` being parsed but unused is current behaviour, recorded here so that
changing it is a deliberate decision rather than an accident.

## 3. Rendering pipeline

### Order of operations

1. Read `skip` patterns from every `kind: KrmGen` file at the top level of the
   source directory (non-recursively). This happens on the **raw YAML**, before
   any template evaluation — the patterns themselves cannot be templated.
2. Merge config-level `skip` patterns with `--skip` flags. Order is preserved,
   duplicates removed, config patterns first.
3. Copy the source directory to a temporary working directory. Every file is
   evaluated as a Go template **except** files matching a skip pattern, which are
   copied byte-for-byte.
4. For each `kind: KrmGen` file at the top level of the working directory
   (non-recursively): run helm for every declared chart, concatenating the
   output in declaration order.
5. If a kustomization file exists anywhere in the working directory, feed the helm
   output into it and let kustomize produce the final result.
6. Write the result to stdout.

### Skip pattern matching

Patterns use `filepath.Match` syntax. Each pattern is tested against **both** the
full relative path and the bare filename, so `*.pfx` matches `certs/prod/cert.pfx`
without needing a directory prefix.

### Kustomization discovery

The working directory is walked recursively for `kustomization.yaml`,
`kustomization.yml` or `kustomization` (case-insensitive). Finding **more than one
is a fatal error** — krmgen will not guess which one you meant.

When a kustomization exists, helm output is written to a file with a generated
name inside the working directory and appended to that kustomization's `resources`
list. The generated filename must never appear in the output.

### Working directory lifecycle

The working directory is created under the system temp directory with mode 0700
and removed after a successful run.

**Known deviation:** when rendering fails, the process exits before cleanup runs,
leaving the working directory — including any rendered secrets — on disk. This is
recorded as current behaviour; it is fixed in phase 3 of the refactoring plan.

## 4. Template functions

Every file copied to the working directory is evaluated as a Go template, unless
it matches a skip pattern. Available functions:

### krmgen

| Function | Arity | Returns |
|---|---|---|
| `krmgenVer` | 0 | Current krmgen version |
| `krmgenGenerated` | 0 | `krmgen-<version>`, intended as a label value |

### Environment

| Function | Arity | Returns |
|---|---|---|
| `argocdEnv <key> [default]` | 1–2 | `ARGOCD_ENV_<key>`, then `ARGOCD_APP_<key>`; error if unset and no default |
| `kubeEnv <key> [default]` | 1–2 | `KUBE_<key>`; error if unset and no default |
| `readF <relpath> [default]` | 1–2 | File contents; path must be relative; error if unreadable and no default |

### Azure

| Function | Arity | Returns |
|---|---|---|
| `azSec <vault> <secret> [version]` | 2–3 | Key Vault secret value |
| `azCert <vault> <cert> [version]` | 2–3 | Key Vault certificate, PEM-encoded |
| `azKey <vault> <key> [version]` | 2–3 | Key Vault key |
| `azPfxKey <vault> <secret> [version]` | 2–3 | Private key extracted from a PKCS12 secret |
| `azPfxCrt <vault> <secret> [version]` | 2–3 | Certificate chain extracted from a PKCS12 secret |
| `azStoreKey <subscription> <resourceGroup> <account>` | 3 | Storage account key |
| `azUaIdClientId <resourceGroup> <name>` | 2 | Client ID of a user-assigned managed identity |
| `toPem <type> <data>` | 2 | Wraps bytes in a PEM block |

`azPfxKey` and `azPfxCrt` forward their arguments to the same secret lookup as
`azSec` before extracting the key or certificate, so they accept the same
optional version argument.

Only `azSec` resolves an omitted version by scanning all versions of the secret
and selecting the latest **enabled** version whose `notBefore` is not in the
future. `azCert` and `azKey` do not perform this scan: omitting the version
argument passes an empty version straight to the Key Vault API, which returns
that service's own notion of the current version, without any client-side
enabled/`notBefore` filtering.

### Sprig

All [sprig](https://masterminds.github.io/sprig/) functions are available, except
`env` and `expandenv`, which are removed so that templates cannot read arbitrary
process environment. Use `argocdEnv` or `kubeEnv` instead.

### Caching

Each Azure provider keeps an in-memory, per-process cache intended to turn two
references to the same resource into one network call. In practice, only one
of the four lookup paths does this correctly:

- **`azSec` without a version** works as intended: `getLatestActiveSecret`
  re-saves the resolved secret under the same no-version key that lookups use,
  so a second reference to the same vault/secret pair is served from cache.
- **`azSec` with an explicit version**, **`azCert`**, and **`azKey`** each
  save the result under the resource ID Azure returns in the response, while
  lookups are keyed by a locally constructed ID built from `<vaultUrl>/<name>/<version>`.
  Azure's real IDs contain an extra path segment (e.g. `.../secrets/<name>/<version>`,
  `.../keys/<name>/<version>`) that the local key never contains, so the two
  never match. The cache entry is written but is unreachable — every reference
  to the same secret/certificate/key re-fetches from Azure.

**Known deviation:** for `azKey` specifically, the unreachable entry is also
the wrong shape — it stores an empty `KeyBundle{}` rather than the fetched
key, so a cache hit (impossible today, but a latent risk of any future fix to
the ID mismatch) would dereference a nil `Key` field and panic. This is
recorded as current behaviour; it is not fixed in this phase.

### Naming note

`azUaIdClientId` is inconsistent with the other Azure functions. Renaming it to
`azClientId`, keeping the old name as an alias, is proposed for phase 3 — where
the function moves to the `cloud-go-templates` library and a rename can be
released together with the new module.

## 5. External tool support matrix

krmgen invokes `helm` and `kubectl` as external binaries. Which binary is used:

| Variable | Effect when set | Effect when unset |
|---|---|---|
| `KRMGEN_HELM_EXECUTABLE` | That path is used as helm | `helm` is looked up in `PATH` |
| `KRMGEN_KUBECTL_EXECUTABLE` | **Not implemented.** Declared but never read | `kubectl` is looked up in `PATH` |

### Supported versions

| Tool | Supported | Verified against |
|---|---|---|
| helm | 3.x, 4.x | v3.21.4, v4.2.4 |
| kubectl | 1.3x with embedded Kustomize 5.x | v1.36.3 / Kustomize v5.8.1 |

### Known differences between helm versions

**helm 4 writes OCI pull progress to stdout; krmgen strips it.** When a chart
is pulled from an OCI registry, helm 4 prints `Pulled: <ref>` and
`Digest: <sha256>` to **stdout**, ahead of the rendered manifests. helm 3 does
not print these lines. krmgen removes this: `templateHelm` in
`internal/helm/processor.go` passes helm's stdout through `stripHelmBanner`,
which removes a **leading contiguous run** of lines starting with `Pulled: `,
`Digest: `, `Signed by: ` or `Chart Hash Verified: `, stopping at the first
line that does not match. Because only a leading run is removed, identical
text appearing inside a rendered manifest (not at the very start of the
output) is preserved — covered by the `stripHelmBanner` unit tests in
`internal/helm/processor_test.go`. Verified directly against
`oci://ghcr.io/stakater/charts/reloader` with both binaries, and again by
running `krmgen generate` against an OCI-chart fixture with
`KRMGEN_HELM_EXECUTABLE` pointed at each binary in turn: both runs exit 0, and
the `Pulled:`/`Digest:` lines are absent from both outputs — the helm 4 output
now starts with `---` / `# Source: ...`, same as helm 3. The two outputs are
still not fully byte-identical: the helm 4 output has three extra blank lines,
one before each `---` document separator between rendered manifests. This is
ordinary helm-version rendering noise unrelated to the OCI banner (both
outputs are otherwise line-for-line the same), and is not something krmgen
processes or strips.

**Kustomize version follows kubectl.** The embedded Kustomize version is whatever
the installed kubectl ships. Two hosts with different kubectl versions can produce
different output from the same input. This is the primary reason the refactoring
plan moves to a pinned library.

### Version detection

krmgen does not currently detect or report the version of the external tools it
invokes. Adding a startup check that records both versions is a requirement for
phase 5, where two backends must be told apart in bug reports.

## 6. Backend parity exceptions

To be completed in Task 6.

## 7. Non-goals

To be completed in Task 6.
