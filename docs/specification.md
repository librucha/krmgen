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

This applies to the `generate` command specifically. Running krmgen with no
arguments, or with `--help`, prints cobra's usage/help text to **stdout** at
exit 0 — that output is not YAML, and the table below does not cover it.

| Stream | Content |
|---|---|
| stdout | Rendered YAML, and nothing else (for a successful `generate` run) |
| stderr | Log messages, warnings, errors |

For `generate`, anything consuming krmgen may redirect stderr to `/dev/null`
and still receive valid YAML on stdout. This separation is a guarantee for
`generate`, not an implementation detail — it does not extend to help output
or other commands.

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
Every matching file is processed, in directory-listing order, and each produces
its own `fmt.Println` output block. This is **not** the same as each pass being
independent: when a kustomization file is also present, every pass runs its own
helm → kustomize step against that same, on-disk, mutated kustomization file —
see the Known deviation in section 3 for what that means with more than one
config file.

The machine-readable contract is [`resources/krmgen-config-schema.json`](../resources/krmgen-config-schema.json).
This schema is documentation only: nothing in the binary loads or validates
against it at runtime (`internal/config/parser.go` unmarshals YAML directly).
It is exercised only by `internal/config/schema_test.go`, which keeps it honest
against what the binary actually accepts.

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
| `helm.charts[].name` | string | no (backend-dependent, see below) | Chart name |
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

**`name` is not enforced, and its effect differs per backend.** Neither krmgen
nor `resources/krmgen-config-schema.json` requires `helm.charts[].name`.
Whether omitting it actually works depends on which backend the chart's `repo`
selects:

- **OCI backend** (`oci://` repo): `name` never reaches the helm command line
  at all (`ociHelmGenerator.addRepoArgs`, `internal/helm/oci-generator.go:51-53`
  appends only `config.RepoUrl`). Verified with a fake-`helm` shim and with a
  real registry (`oci://ghcr.io/stakater/charts/reloader`, no `name` in
  config): both exit 0 and render normally.
- **HTTP(S) repo backend**: `name` is passed, via
  `repoHelmGenerator.addRepoArgs` (`internal/helm/repo-generator.go:39-41`), as
  the value following a `--release-name` flag. Real helm's `--release-name` is
  a boolean flag with no value of its own ("use release name in the
  output-dir path"), so the chart name that follows it is consumed by helm as
  the ordinary positional `CHART` argument instead — a coincidental, not
  designed, way of getting the chart name onto the command line. Omitting
  `name` here **does** break rendering: verified against a real repository
  (`https://charts.jetstack.io`, no `name` in config) — helm exits 1 with
  `Error: chart "" version "v1.14.0" not found in https://charts.jetstack.io
  repository`. The failure surfaces as a helm error, not a krmgen or schema
  validation error.

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
   (non-recursively, processed in directory-listing order), run the following
   **as one pass, per config file** — not as two global phases:
   1. Run helm for every chart declared in that config file, concatenating
      the output in declaration order (`config.ProcessConfig`,
      `internal/config/processor.go:12-18`).
   2. If a kustomization file exists anywhere in the working directory, feed
      that pass's helm output into it and let kustomize produce that pass's
      result (`internal/config/processor.go:19-24`). This kustomize step runs
      **inside the per-config loop**, once per `kind: KrmGen` file, against
      the same on-disk kustomization file every time — see the Known
      deviation below.
   3. Print that pass's result to stdout.
5. (There is no separate global "run kustomize once" phase — step 4.2 above
   is it, and it happens once per config file.)

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

**Known deviation:** this append happens **on disk**, once per `kind: KrmGen`
config file (see Order of operations, step 4.2), and nothing removes a
generated resource file or its `resources` entry once a pass has finished.
With more than one config file and a shared kustomization, every pass after
the first sees the kustomization file already carrying every earlier pass's
generated resource — so its own kustomize step processes the accumulation of
all passes so far, not just its own.

Reproduced directly: two `kind: KrmGen` files plus one `kustomization.yaml` in
the same source directory, each declaring a chart that helm renders as a
`ConfigMap` with the same name. The first pass succeeds; the second pass's
kustomize invocation fails fatally:

```
level=fatal msg="run kubectl kustomize failed error: exit status 1 reason: error:
accumulating resources: accumulation err='merging resources from '<uuid>.yml': may
not add resource with an already registered id: ConfigMap.v1.[noGrp]/<name>.[noNs]':
must build at directory: '<workdir>/<uuid>.yml': file is not directory"
```

When the two passes' resources do **not** collide by identity, the run does
not fail — it exits 0, but the second pass's output block silently includes
the first pass's resources too (verified with distinctly-named ConfigMaps from
each pass: the second pass's printed block contained both). Either way, "each
[config file] produces its own output block" (section 2) does not hold once a
kustomization file is shared by more than one config file. This is recorded as
current behaviour; a source directory should have at most one `kind: KrmGen`
file when a kustomization file is also present, until this is addressed.

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

`readF` resolves `<relpath>` against krmgen's **process working directory at
invocation time** (`os.ReadFile(relPath)`, `internal/template/files/files-provider.go:26`,
no base directory is joined) — **not** the source directory passed to
`krmgen generate <path>`. Running `krmgen generate /some/src` from inside
`/some/src` and from anywhere else resolves the same `readF "data.txt"` call
against two different files. Verified: invoked with cwd set to the source
directory, the call succeeds and returns the file's contents; invoked with cwd
set elsewhere, the same call fails to find the file and falls back to the
`[default]` argument (or errors, if none was given). This has gone unnoticed
in the primary deployment target — under ArgoCD's Config Management Plugin
protocol, the CMP process's working directory is set to the application
directory, which is also krmgen's source directory, so the two happen to
coincide there.

### Azure

| Function | Arity | Returns |
|---|---|---|
| `azSec <vault> <secret> [version]` | 2–3 | Key Vault secret value |
| `azCert <vault> <cert> [version]` | 2–3 | Key Vault certificate, PEM-encoded |
| `azKey <vault> <key> [version]` | 2–3 | RSA modulus, PEM-encoded under an `"<KTY> PRIVATE KEY"` header (see note below) |
| `azPfxKey <vault> <secret> [version]` | 2–3 | Private key extracted from a PKCS12 secret |
| `azPfxCrt <vault> <secret> [version]` | 2–3 | Certificate chain extracted from a PKCS12 secret |
| `azStoreKey <subscription> <resourceGroup> <account>` | 3 | Storage account key |
| `azUaIdClientId <resourceGroup> <name>` | 2 | Client ID of a user-assigned managed identity |
| `toPem <type> <data>` | 2 | Wraps bytes in a PEM block |

**`azKey` does not return a private key, despite the PEM header.** `wrapKey`
(`internal/template/azure/key/azure-key-provider.go:84-90`) builds the PEM
block from `key.Key.N` — the RSA **modulus**, which Key Vault's `GetKey`
response always includes and which is public information — under a header of
`fmt.Sprintf("%s PRIVATE KEY", *key.Key.Kty)` (e.g. `RSA PRIVATE KEY` for an
RSA key). The emitted PEM is therefore a public value wearing a private-key
label; it is not usable as a private key and does not contain one. A future
reimplementation must reproduce this exact (mislabeled) output, not what the
header claims.

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

Five Azure provider functions each keep an in-memory, per-process cache
(a plain `map`), across six lookup paths in total — `azSec` has two, one for an
explicit version and one without. Two references to the same resource cause one
network call; the cache is never invalidated during a run, and it does not
persist between runs.

Every path keys the cache by a locally constructed ID, and saves the fetched
result under that same ID:

| Function | Cache key |
|---|---|
| `azSec` (explicit version) | `<vaultUrl>/<name>/<version>` |
| `azSec` (no version) | `<vaultUrl>/<name>/` and the resolved `<vaultUrl>/<name>/<version>` |
| `azCert` | `<vaultUrl>/<name>/<version>` |
| `azKey` | `<vaultUrl>/<name>/<version>` |
| `azStoreKey` | `<subscriptionID>/<resourceGroup>/<account>` |
| `azUaIdClientId` | `<resourceGroup>/<name>` |

`azSec` without a version stores the result twice: under the no-version key, so
a later version-less reference skips the version listing entirely, and under the
resolved version's key, so a later reference that names that version hits the
same entry.

Because the key is built from the arguments rather than from the resource ID
Azure returns, a reference that omits the version and one that names the same
version resolve to the same entry only for `azSec`. For `azCert` and `azKey`,
a version-less reference and an explicit-version reference to the same
underlying resource are two separate cache entries, and therefore two
network calls.

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

### Invocation

This is the command line krmgen actually builds for `helm template`
(`templateHelm`, `internal/helm/processor.go:50-83`, plus each generator's
`addRepoArgs`/`addCredentials`). Verified with a fake `helm` executable
(pointed to via `KRMGEN_HELM_EXECUTABLE`) that logs its argv, for both
backends, and cross-checked against real registries. This is the single most
parity-critical fact in this document: phase 5's go/no-go for the embedded
helm library is a measured-parity decision against this exact invocation.

**Argument order, both backends**, built in this sequence:

1. `template`
2. the release name (positional, **may be empty** — `config.ReleaseName` is
   used as-is, with no validation; verified: an empty `releaseName` produces
   `helm template  --include-crds ...` with an empty positional argument, and
   helm accepts it)
3. `--include-crds` — passed **unconditionally**, on every invocation,
   regardless of config
4. `--version <version>`, only if `version` is set
5. `--namespace <namespace>`, only if `namespace` is set
6. backend-specific repo args (see below)
7. credential args (see below), only if credentials are available and not
   suppressed
8. `--values <file>` for `valuesFile` if set, **then** `--values <file>` for
   `valuesInline` if set — both flags appear when both fields are set, in
   that order (`getValuesArgs`, `internal/helm/processor.go:116-136`); a
   `valuesInline` values file is a generated temp file, written into the
   working directory

**OCI backend** (`oci://` repo) appends exactly one argument for the repo:
the raw `repo` URL (`ociHelmGenerator.addRepoArgs`,
`internal/helm/oci-generator.go:51-53`). Example observed argv:

```
helm template myrelease --include-crds --version 1.2.3 --namespace myns \
  oci://registry.example.com/helm/mychart \
  --username bob --password secret \
  --values /tmp/.../helm-values-myrelease-<uuid>
```

**HTTP(S) repo backend** appends `--repo <url> --release-name <name>`
(`repoHelmGenerator.addRepoArgs`, `internal/helm/repo-generator.go:39-41`).
Example observed argv:

```
helm template myrelease --include-crds --version 2.0.0 --namespace myns \
  --repo https://charts.example.com/repo --release-name mychart \
  --username alice --password pw123 \
  --values /tmp/.../helm-values-myrelease-<uuid>
```

**`--kube-version` and `--api-versions` are never passed**, on either
backend — confirmed by reading `internal/helm/processor.go` and both
generators, and by inspecting captured argv from the fake-helm shim across
every scenario tested. `.Capabilities` inside chart templates therefore
resolves to whatever the invoked helm binary defaults to for a client-only
`template` run, which differs between helm versions and is not something
krmgen controls or records. This is a named risk for phase 5 and is tracked
under Known differences between helm versions, below.

**Credentials.** `credentialsArgs` (`internal/helm/generator.go:52-72`) builds
`--username <value> --password <value>` (either flag omitted if its value is
empty) from, per field: `config.Username`/`config.Password` if set, else
`KRMGEN_HELM_USERNAME`/`KRMGEN_HELM_PASSWORD`. Setting
`ignoreCredentials: true` on a chart makes `credentialsArgs` return no args at
all for that chart — config values and env var fallbacks are both suppressed,
unconditionally. `credentialsProvided` (used to decide whether to run
`helm registry login` for the OCI backend, and whether to append credential
args at all) is simply "would `credentialsArgs` return anything". For the OCI
backend, when credentials are provided, krmgen runs `helm registry login
<registry-host> --username <u> --password <p>` **before** the `template`
call (`ociHelmGenerator.login`, `internal/helm/oci-generator.go:37-45`);
verified in the captured argv above (`registry login registry.example.com
--username bob --password secret` precedes the `template` invocation). The
HTTP repo backend never calls login (`repoHelmGenerator.login` is a no-op);
its credentials are carried only on the `template` command itself.

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

**`.Capabilities` is whatever the invoked helm defaults to.** krmgen never
passes `--kube-version` or `--api-versions` (see Invocation, above), so a
chart that branches on `.Capabilities.KubeVersion` or
`.Capabilities.APIVersions` renders differently depending only on which helm
binary is installed on the host — helm versions ship different built-in
defaults for a client-only `template` run. This is invisible today because it
never varies within a single host, but it is a named risk for phase 5: an
embedded helm library must default to the same `.Capabilities` values as
whichever external helm version it is being measured for parity against, or
the two backends will disagree on any chart that reads `.Capabilities`.

### Version detection

krmgen does not currently detect or report the version of the external tools it
invokes. Adding a startup check that records both versions is a requirement for
phase 5, where two backends must be told apart in bug reports.

## 6. Backend parity exceptions

The refactoring plan introduces a second, embedded backend for both helm and
kustomize, selected by the absence of `KRMGEN_HELM_EXECUTABLE` and
`KRMGEN_KUBECTL_EXECUTABLE`. Both backends are measured against the same golden
files. Where they cannot agree, the difference is recorded here rather than
treated as a defect:

| Capability | External binary | Embedded library |
|---|---|---|
| Helm plugins | Available | **Never available** |
| Helm post-renderers | Available | **Never available** |
| Kustomize version | Whatever kubectl ships | Pinned in `go.mod` |
| Helm version | Whatever the host has | Pinned in `go.mod` |

Users depending on helm plugins or post-renderers must set
`KRMGEN_HELM_EXECUTABLE` and keep the external backend.

## 7. Non-goals

krmgen deliberately does not:

- **Talk to a cluster.** All rendering is offline. Helm is always run in
  client-only mode; `lookup` in charts returns nothing.
- **Apply anything.** Output goes to stdout; deploying it is the caller's job.
- **Manage secret lifecycle.** Template functions read secrets from Azure at
  render time; krmgen does not create, rotate or write them back.
- **Provide a plugin system.** Template functions are compiled in.
- **Guarantee stable output across versions of the embedded tools.** Upgrading
  the pinned helm or kustomize may change output; such changes are release-noted.
