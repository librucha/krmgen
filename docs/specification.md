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

A failure to remove the working directory during cleanup never affects the exit
code. It is reported as a stderr warning (see Working directory lifecycle, below),
not promoted to a returned error, so a run that rendered successfully still exits 0
and a run that failed still exits 1 for its own reason.

krmgen currently does not distinguish error classes by exit code. Callers must not
rely on a specific non-zero value beyond "non-zero means failure".

### Error output

A failing run writes the cause to stderr and exits with code 1:

    Error: <what failed>

No timestamp, no log level, and no usage block. An unrecognised command is
the one case that adds a second line, cobra's own `Run 'krmgen --help' for
usage.` suggestion.

Before phase 5 the format depended on which package raised the error - the
kustomize processor used logrus (`time=... level=fatal msg="..."`) and the
rest used the standard library's `log` (`2026/08/27 00:23:33 ...`). Each
individual message's wording is otherwise unchanged by the switch; the one
exception is a typo fix (`crating directory` to `creating directory`). The
stderr *line* changed more than that, though: every error that used to
`log.Fatal` deep inside a call chain now gets wrapped once per layer it
returns through, so what reaches stderr is a chain like
`Error: processing config file <path> failed error: <cause>`, not just
`<cause>` on its own. Only a caller matching on the innermost cause's
substring is unaffected by that extra wrapping.

Callers matching on stderr should match on the message, not on the line shape.

#### Rendered resources are never echoed into an error

kustomize reports a resource it cannot serialize to YAML (`ResMap.AsYaml`,
`sigs.k8s.io/kustomize/api/resmap/reswrangler.go`) by prefixing the real
reason with a `%#v` dump of that resource's entire map. On a generated
`Secret` that dump is every value in clear text, and krmgen used to wrap it
verbatim — so a failing render printed the secrets it had just built to
stderr, and under ArgoCD into the application's retained sync log. The same
data is kept at mode 0600 on disk (see Working directory lifecycle), so
printing it contradicted the rest of the tool.

Neither backend propagates that dump any more:

- **Embedded.** `internal/kustomize/builder_krusty.go` re-runs the
  per-resource serialization itself, and reports the offending resource's
  kind, namespace and name plus the parser's reason:
  `rendering kustomize output failed: resource Secret demo could not be
  serialized to YAML: yaml: did not find expected ',' or '}'`.
- **External.** kubectl embeds the same kustomize and printed the same dump.
  Across a subprocess boundary there is no resource list to inspect, so
  `internal/kustomize/builder_kubectl.go` drops kubectl's stderr entirely
  whenever it carries the dump, replacing it with a note pointing at the
  embedded backend. Any other kubectl stderr is passed through unchanged.

This is the one error path where the two backends deliberately differ in
wording; `TestError_RenderFailureDoesNotLeakSecretValues` (`test/golden/errors_test.go`)
asserts what must hold for both — exit code 1, and no secret value on either
stream.

The route the report reached this through — a multi-line value in a generator
env file — is now rejected before a backend runs (see Generator env files are
validated). The redaction still stands for every other way a rendered resource
can fail to serialize, such as a key long enough for kyaml to fold the line.

#### URL credentials are masked in every error

A kustomization may pull a remote base over HTTPS with the credential in the
URL, typically resolved from a key vault by a template so the repository itself
stores no secret:

    resources:
      - https://user:{{ azSec "vault" "git-password" }}@dev.azure.com/org/proj/_git/repo/apps/x?ref=main

krmgen resolves templates before rendering, so by fetch time the URL carries
the real token. When the fetch fails — a transient 503 upstream is enough —
kustomize echoes the resolved URL verbatim, twice: once naming the resource it
could not accumulate, once quoting the git command line it ran. Go's own
`net/http` redacts the copy it formats; the two kustomize formats did reach
stderr in full, and under ArgoCD that is the application's retained sync log.

Every error leaving `generate` therefore passes through `redact.Error`
(`internal/redact`), which rewrites the password of every URL in the message to
`***`:

    accumulating resources from 'https://user:***@git.invalid/org/_git/repo?ref=main':
    failed to run '/usr/bin/git fetch --depth=1 https://user:***@git.invalid/org/_git/repo main'

The repository stays identifiable, so the error is still worth printing. A URL
carrying only a username is left alone — a username is not a credential. The
masked error is a new error rather than a wrapper, so the original is not one
`Unwrap` away; an error with nothing to mask is returned untouched and keeps
its identity for `errors.Is`. Both backends are covered:
`TestError_RemoteBaseCredentialsAreMasked` (`test/golden/errors_test.go`)
asserts it for each.

Two exposures this does **not** close, both outside krmgen's reach:

- **The credential is in git's argv.** kustomize fetches a remote base by
  running `git fetch --depth=1 <resolved URL> <ref>`, so for the duration of
  the fetch the token is visible to anything that can read the process table on
  that host. Closing it would need kustomize to use `GIT_ASKPASS` or a
  credential helper.
- **kustomize writes to `os.Stderr` directly**, bypassing the returned error
  (`internal/target/kusttarget.go`). In the pinned version those messages are
  fixed deprecation strings holding no user data, so nothing leaks through that
  channel today — but a message that carried a URL would not be masked.

The same shape exists in the external helm backend, which passes
`--username`/`--password` on helm's command line (`internal/helm/generator.go`)
and so puts them in argv too; the embedded backend keeps them in memory (see
Backend parity exceptions).

A failure of the render itself is the only known way rendered content reaches
an error. Go's `text/template` reports an execution failure with the template
expression and its position, never the output buffer, so the
`template evaluation of file %s failed` error in `cmd/generate.go` does not
carry partially rendered output.

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

**Known deviation:** the search is recursive but the build is not. Whichever
kustomization the walk finds, the selected backend (see Section 5) always
builds against the **root** of the working directory. A kustomization that lives only in a
subdirectory is therefore discovered, has the generated resource file appended
to it, and then goes unused — the build fails at the root instead:

```
error: unable to find one of 'kustomization.yaml', 'kustomization.yml' or
'Kustomization' in directory '<workDir>'
```

That is the external backend's wording. The default embedded backend reports
the same underlying kustomize error without the `error: ` prefix, wrapped in
its own text:

```
run kustomize failed: unable to find one of 'kustomization.yaml', 'kustomization.yml' or 'Kustomization' in directory '<workDir>'
```

Exit code 1. In practice a kustomization must sit at the top level of the source
directory; nesting it silently costs a run rather than being rejected up front.

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
kustomize invocation fails, and the process exits 1 with:

```
Error: processing config file <workdir>/krmgen-b.yaml failed error: run kustomize
failed: accumulating resources: accumulation err='merging resources from
'<uuid>.yml': may not add resource with an already registered id:
ConfigMap.v1.[noGrp]/<name>.[noNs]': must build at directory:
'<workdir>/<uuid>.yml': file is not directory
```

The `run kustomize failed: ...` wrapper shown above is the default embedded
backend's wording; the external `kubectl kustomize` backend wraps the same
underlying kustomize error differently, as `run kubectl kustomize failed
error: ...`. Both raise the same underlying kustomize error — verified by
`TestGolden_BothBackendsAgreeOnErrors` (`test/golden/harness_test.go`), see
Section 6.

When the two passes' resources do **not** collide by identity, the run does
not fail — it exits 0, but the second pass's output block silently includes
the first pass's resources too (verified with distinctly-named ConfigMaps from
each pass: the second pass's printed block contained both). Either way, "each
[config file] produces its own output block" (section 2) does not hold once a
kustomization file is shared by more than one config file. This is recorded as
current behaviour; a source directory should have at most one `kind: KrmGen`
file when a kustomization file is also present, until this is addressed.

### Generator env files are validated

`secretGenerator` and `configMapGenerator` can read key/value pairs from an env
file (`envs:`, or the deprecated singular `env:`). That file is in the
`.properties` format, which has no way to carry a newline: kustomize truncates
a multi-line value at the first line break and reads every following line as a
further key with an empty value. So

    creds={
      "private_key": "TOP-SECRET",
      "id": "x"
    }

becomes a Secret whose `creds` key holds base64 of `{` and whose remaining keys
are the fragments of the credential, unencoded — visible in
`kubectl get secret -o yaml` without a base64 step. kustomize intends to reject
such keys (`keyValuesFromLine` calls `Validator.IsEnvVarName` on each one) but
the implementation behind that call is an unimplemented stub returning nil
(`FieldValidator.IsEnvVarName`,
`sigs.k8s.io/kustomize/api/internal/validate/fieldvalidator.go`), so nothing
catches it. krmgen is the last place that can: it is where the value arrives
from a key vault via `azSec`.

Before rendering, krmgen therefore rejects any env-file line whose key is not a
valid ConfigMap/Secret key — `[-._a-zA-Z0-9]+`, the format the API server
accepts (`internal/kustomize/envfiles.go`):

    Error: processing config file <path> failed error: secretGenerator "demo":
    env file "resources/secrets.properties" line 2 is read as a key, but it is
    not a valid ConfigMap/Secret key. ...

The offending line is a fragment of the value, so the message names the file and
the line rather than quoting it. The fix is to write the value to its own file
and reference it from the generator's `files:` field, which carries the content
verbatim.

The rule is the API server's key format, not the stricter env-var-name format,
so it cannot reject a generator that renders to a manifest the cluster would
have accepted — a key starting with a digit, for instance, keeps working. Only
the kustomization krmgen builds is checked; env files reachable through a nested
kustomization are resolved by kustomize itself and never pass through krmgen.
The check runs before a backend is selected, so it applies to the external
kubectl backend as well.

### Working directory lifecycle

The working directory is created under the system temp directory with mode 0700
(via `os.MkdirTemp`, which applies that mode itself) and removed when the run
ends, whether it succeeds or fails. As of phase 5, cleanup is registered as
soon as the directory exists, so a failure partway through rendering no longer
leaves the working directory — including any rendered secrets — on disk. This
is covered by `TestError_WorkingDirectoryRemovedOnFailure`
(`test/golden/errors_test.go`), which runs a failing scenario under its own
`TMPDIR` and asserts no `krmgen*` directory remains in it. If the removal
itself fails, that is not promoted to a returned error — a render that
already succeeded stays a success — but it is not silent either: krmgen
prints a stderr warning naming the path left behind, since it may still hold
rendered secrets.
Everything krmgen writes *inside* that directory — copied and template-evaluated
files, any subdirectories created while copying the source tree, per-chart
`helm-values-<release>-<uuid>` files generated from `valuesInline`
(`internal/helm/processor.go`), and the `<uuid>.yml` file kustomize's resources
are collected into (`internal/kustomize/processor.go`) — is
likewise restricted to the owning user: subdirectories are created with mode
0700 and files with mode 0600 (`cons.DirPerm` / `cons.FilePerm`,
`internal/utils/perm.go`), because those files may hold rendered templates,
including secrets pulled from a key vault by the `azSec` family of functions.
Before phase 5, files were written with `os.ModePerm` (0777) or `0666`, and
subdirectories with `0750` — world- or group-readable modes that a rendered
secret should never have had.

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

These functions are provided by the `github.com/librucha/cloud-go-templates`
library (package `azure`), not by code in this repository. krmgen depends on
the library and merges its `Provider.FuncMap()` into the template function set
at `internal/template/template.go`.

| Function | Arity | Returns |
|---|---|---|
| `azSec <vault> <secret> [version]` | 2–3 | Key Vault secret value |
| `azCert <vault> <cert> [version]` | 2–3 | Key Vault certificate, PEM-encoded |
| `azKey <vault> <key> [version]` | 2–3 | RSA modulus, PEM-encoded under an `"<KTY> PRIVATE KEY"` header (see note below) |
| `azPfxKey <vault> <secret> [version]` | 2–3 | Private key extracted from a PKCS12 secret |
| `azPfxCrt <vault> <secret> [version]` | 2–3 | Certificate chain extracted from a PKCS12 secret |
| `azStoreKey <subscription> <resourceGroup> <account>` | 3 | Storage account key |
| `azUserIdentityClientId <resourceGroup> <name>` | 2 | Client ID of a user-assigned managed identity |
| `toPem <type> <data>` | 2 | Wraps bytes in a PEM block |

`azUaIdClientId` remains registered as a deprecated alias of
`azUserIdentityClientId` — see Naming note below. It is krmgen's own alias,
registered in `internal/template/template.go`, not something the library
exposes.

**`azKey` does not return a private key, despite the PEM header.** The
library's key provider builds the PEM block from the RSA **modulus**, which
Key Vault's `GetKey` response always includes and which is public
information, under a header of `"<KTY> PRIVATE KEY"` (e.g. `RSA PRIVATE KEY`
for an RSA key). The emitted PEM is therefore a public value wearing a
private-key label; it is not usable as a private key and does not contain
one. This behaviour is unchanged by the move to the library.

`azPfxKey` and `azPfxCrt` forward their arguments to the same secret lookup as
`azSec` before extracting the key or certificate, so they accept the same
optional version argument.

Only `azSec` resolves an omitted version by scanning all versions of the secret
and selecting the latest **enabled** version whose `notBefore` is not in the
future. `azCert` and `azKey` do not perform this scan: omitting the version
argument passes an empty version straight to the Key Vault API, which returns
that service's own notion of the current version, without any client-side
enabled/`notBefore` filtering.

#### Error behaviour of unusual Azure responses

The library's port of these functions converted several unchecked pointer
dereferences — panics that took the whole krmgen process down — into
returned errors. The record of this phase originally claimed "exactly one
deliberate behaviour change"; there are four, listed here because that
original claim was wrong (see the Naming note's sibling document,
`docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md`, corrected
alongside this section). All four are improvements over panicking and none
has been reverted.

1. **`azStoreKey` with an empty `Keys` list** (sanctioned in the original
   plan). The storage account has no access keys at all. Previously indexed
   `Keys[0]` unconditionally and panicked; now returns an error naming the
   account (`cloud-go-templates/azure/storage.go`,
   `TestStorageKeyFunc_NoKeysIsAnErrorNotAPanic`).
2. **`azStoreKey` with `Keys[0].Value == nil`.** Azure returned a key entry
   with no value. Previously dereferenced the nil pointer and panicked; now
   returns the same "no keys" error as case 1
   (`TestStorageKeyFunc_NilFirstKeyValueIsAnError`).
3. **`azSec` with `secret.Value == nil`.** Key Vault returned a secret
   version with no value. Previously dereferenced the nil pointer and
   panicked; now returns an error naming the secret and vault
   (`cloud-go-templates/azure/secrets.go`, `TestSecret_NilValueIsAnError`).
4. **`azUserIdentityClientId` (`azUaIdClientId`) with a missing client ID.**
   This is the only one of the four that changes what a *successful* render
   produces, not just what a panic produces. `identity.Properties == nil` was
   already a panic before this phase and is now an error
   (`TestUserIdentityClientIDFunc_NilPropertiesIsAnError`) — that part matches
   the "panic to error" pattern of cases 1–3. But `identity.Properties.ClientID
   == nil` (Properties present, ClientID absent) previously let a nil pointer
   flow into the `any` the function returned, which `text/template` printed as
   the literal string `<nil>` — the render **succeeded** with that string in
   the output. It is now a hard error naming the identity and resource group
   (`cloud-go-templates/azure/identity.go`,
   `TestUserIdentityClientIDFunc_NilClientIDIsAnError`). The original phase 3
   plan claimed "the rendered output does not change" for this function; that
   was true for the `Properties == nil` panic case but false for the nil
   `ClientID` case, where output changes from `<nil>` to a failed render. That
   was a plan defect, not an implementation defect — the implementation is
   correct and preferred.

A fifth unchecked dereference, in `azKey`'s PEM-wrapping (`wrapKey` in
`cloud-go-templates/azure/keys.go`, dereferencing `key.Key` and
`*key.Key.Kty`), was found and fixed the same way during the same phase's
fix wave: a Key Vault response with no key material, or a key with no
recorded key type, now returns an error naming the key and vault instead of
panicking (`TestKeyFunc_NoKeyMaterialIsAnError`,
`TestKeyFunc_NoKeyTypeIsAnError`). This one was never reachable through
krmgen's golden suite either, same as the four above.

### Sprig

All [sprig](https://masterminds.github.io/sprig/) functions are available, except
`env` and `expandenv`, which are removed so that templates cannot read arbitrary
process environment. Use `argocdEnv` or `kubeEnv` instead.

### Caching

Caching is the library's responsibility, not krmgen's. Each `azure.Provider`
(one per krmgen process — see `internal/template/template.go`, built once
behind a `sync.Once`) owns an in-memory cache, a plain map guarded by a mutex
(`azure/cache.go`). The cache key is always built from the calling function's
own arguments, never from the resource ID Azure returns. Entries live for the
lifetime of the provider — i.e. for the whole `krmgen generate` run — and are
never invalidated or evicted during that run; nothing persists between runs.

Azure **clients** are deduplicated per key: `Provider.client` stores one
`*clientEntry` behind its own `sync.Once` for each client kind
(`azure/provider.go`), so two goroutines building, say, the secrets client for
the same vault wait for and share that one build, while building clients for
different vaults or kinds never blocks across keys. The **cache** gives no
equivalent guarantee for network calls. `cache.get` and `cache.put` are two
independent, briefly mutex-guarded map operations (`azure/cache.go`); every
cached function (e.g. `Secret` in `azure/secrets.go`, and equivalently
`certificate`, `key`, `StorageKeyFunc`, `UserIdentityClientIDFunc`) calls
`cache.get`, and on a miss performs the actual Azure network call **outside
any lock**, only calling `cache.put` once the response comes back. There is
no in-flight request deduplication (no singleflight): two goroutines racing
on the same uncached key each pass the miss check and each issue their own
network call; both then write the same value back under the same key. That
is redundant traffic, not a correctness problem, but it is not "one network
call" — a false guarantee here would matter to whoever measures or reimplements
this behaviour later.

A single caller making two *sequential* references to the same resource with
the same arguments causes one network call: the second finds the entry the
first call's `put` left behind. Because the key is built from arguments
rather than from the resolved resource ID, a version-less reference and an
explicit-version reference to the same underlying secret, certificate or key
are two separate cache entries (and therefore two network calls, even made
sequentially) unless the library's own per-function logic folds them together
— see the library's `azure` package for the exact per-function key shapes.

### Naming note

`azUaIdClientId` read as an abbreviation nobody could expand on sight. It was
renamed to **`azUserIdentityClientId`** in phase 3 (2026-08-25), when the
function moved to the `cloud-go-templates` library and the rename shipped
with the new module. krmgen keeps `azUaIdClientId` registered as a deprecated
alias of the same function (`internal/template/template.go`), so existing
configurations continue to work; the library itself exposes only the new
name.

The name is deliberately explicit about *user*-assigned, because Azure's two
managed-identity kinds cannot share one function. Both expose the same three
properties — client ID, principal ID, tenant ID — but they are addressed
differently, as the `armmsi` SDK makes plain:

| | user-assigned | system-assigned |
|---|---|---|
| Client | `NewUserAssignedIdentitiesClient(subscriptionID, …)` | `NewSystemAssignedIdentitiesClient(…)` — no subscription |
| Call | `Get(ctx, resourceGroup, name)` | `GetByScope(ctx, scope)` |
| Address | resource group plus name | the host resource's full ARM ID |

Folding both into one function would mean dispatching on argument count, and a
reader could not tell the two modes apart at the call site. The naming therefore
leaves room for a symmetric family:

```
azUserIdentityClientId    <resourceGroup> <name>
azUserIdentityPrincipalId <resourceGroup> <name>     (role assignments)
azSystemIdentityClientId  <scope>                    (if it is ever needed)
```

In practice the user-assigned form is what KRM output needs: the
`azure.workload.identity/client-id` annotation requires a federated credential,
and federated credentials exist only under
`Microsoft.ManagedIdentity/userAssignedIdentities/…` — the SDK offers no
system-assigned equivalent.

## 5. External tool support matrix

krmgen invokes neither `helm` nor `kubectl` as an external binary by default.
Both are opt-in: setting `KRMGEN_HELM_EXECUTABLE` selects the external `helm`
binary (`selectRenderer`, `internal/helm/renderer.go`); setting
`KRMGEN_KUBECTL_EXECUTABLE` selects `kubectl kustomize` (`selectBuilder`,
`internal/kustomize/builder.go`). Left unset, both render through the library
compiled into krmgen instead — helm via `helm.sh/helm/v4/pkg/action`
(`sdkRenderer`, `internal/helm/renderer_sdk.go`), kustomize via
`sigs.k8s.io/kustomize/api` (`krustyBuilder`,
`internal/kustomize/builder_krusty.go`). Which binary is used:

| Variable | Effect when set | Effect when unset |
|---|---|---|
| `KRMGEN_HELM_EXECUTABLE` | That path is used as helm | The helm library compiled into krmgen renders |
| `KRMGEN_KUBECTL_EXECUTABLE` | That path is used as kubectl, and kustomize rendering goes through it | The kustomize library compiled into krmgen renders |

`KRMGEN_HELM_EXECUTABLE` set to the empty string is treated the same as
unset — `selectRenderer` (`internal/helm/renderer.go`) still selects the
embedded library, consistent with how `selectBuilder` treats an empty
`KRMGEN_KUBECTL_EXECUTABLE`. `helmExecutable()`
(`internal/helm/processor.go:16-25`) does have its own fallback to a `helm`
found on `PATH` when the variable is unset, but that branch is unreachable in
the current call graph: both of its callers (`ociHelmGenerator.login`,
`binaryRenderer.Render`) are reached only once `selectRenderer` has already
chosen the external backend, which by construction only happens when the
variable names a non-empty path.

**Helm plugins and post-renderers are available only on the external
binary.** Both run through the `helm` executable itself — a plugin is a
subcommand `helm` shells out to, a post-renderer is a binary `helm template`
pipes its output through — and neither has an equivalent when krmgen calls
`helm.sh/helm/v4/pkg/action` directly. This is permanent, not a gap phase 6
is expected to close later; see Section 6.

krmgen embeds `sigs.k8s.io/kustomize/api` v0.21.1 (pinned in `go.mod`).
Parity between the embedded backend and the external `kubectl` backend — see
Section 6 — is enforced by `TestGolden_BothBackendsAgree` and
`TestGolden_BothBackendsAgreeOnErrors` (`test/golden/harness_test.go`) and was
last verified against kubectl v1.36.3, which embeds Kustomize v5.8.1.

krmgen embeds `helm.sh/helm/v4` v4.2.4 (pinned in `go.mod`; see
`anchorHelmLibraryVersion`, `test/golden/versions_test.go`, which asserts this
version is what actually ships in the built binary). Parity between the
embedded library and the external `helm` binary is enforced by
`TestGolden_BothHelmRenderersAgree` (`test/golden/harness_test.go`) and its
OCI-scenario sibling `TestOci_BothHelmRenderersAgree`
(`test/golden/oci_test.go`, behind the `oci` build tag), and was last verified
against the reference external helm build named by `anchorHelmVersion`
(`test/golden/versions_test.go`), v4.2.4+g3900f43.

### Invocation

**This section describes the external binary backend only** (active when
`KRMGEN_HELM_EXECUTABLE` is set). The embedded library backend never builds
an argv at all — it drives the helm Go API directly (`sdkRenderer.Render`,
`internal/helm/renderer_sdk.go`) — so nothing below applies to it.

This is the command line krmgen builds for `helm template` on that backend
(`binaryRenderer.Render`, `internal/helm/renderer_binary.go:18-55`, plus each
generator's `addRepoArgs`/`addCredentials`). Verified with a fake `helm`
executable (pointed to via `KRMGEN_HELM_EXECUTABLE`) that logs its argv, and
cross-checked against real registries. This was the single most
parity-critical fact during phase 6: the go/no-go decision for the embedded
helm library was a measured-parity decision against this exact invocation
(`TestGolden_BothHelmRenderersAgree`, `test/golden/harness_test.go`, which
runs both backends against the same scenarios and requires byte-identical
stdout). Phase 6 is complete; the embedded library has been the default
since task 4 of that phase.

**Argument order, both backends**, built in this sequence:

1. `template`
2. the release name (positional — `config.ReleaseName` is used as-is, with no
   validation of its own; krmgen does still produce
   `helm template  --include-crds ...` with an empty positional argument when
   `releaseName` is unset, but helm does **not** accept it: measured on the
   anchor v4.2.4 (`test/golden/versions_test.go`), both backends exit 1 with
   `release name "": no name provided`)
3. `--include-crds` — passed **unconditionally**, on every invocation,
   regardless of config
4. `--version <version>`, only if `version` is set
5. `--namespace <namespace>`, only if `namespace` is set
6. backend-specific repo args (see below)
7. credential args (see below), only if credentials are available and not
   suppressed
8. `--values <file>` for `valuesFile` if set, **then** `--values <file>` for
   `valuesInline` if set — both flags appear when both fields are set, in
   that order (`getValuesArgs`, `internal/helm/processor.go:84-94`); a
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

**`--kube-version` and `--api-versions` are never passed** on this backend —
confirmed by reading `internal/helm/renderer_binary.go` and both generators,
and by inspecting captured argv from the fake-helm shim across every scenario
tested. `.Capabilities` inside chart templates therefore
resolves to whatever the invoked helm binary defaults to for a client-only
`template` run, which differs between helm versions and is not something
krmgen controls or records. This was a named risk during phase 6 and remains
tracked under Known differences between helm versions, below.

**Credentials.** `credentialsArgs` (`internal/helm/generator.go:89-99`) builds
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
| helm (external binary backend only) | 3.8.0 and later, including 4.x | v3.8.2, v3.21.4, v4.2.4 |
| helm (embedded library backend) | pinned in `go.mod`, not a range | v4.2.4 |
| kubectl (external kustomize backend only) | any release providing `kubectl kustomize` | v1.36.3 / Kustomize v5.8.1 |

**Why helm 3.8.0 is the floor.** This floor applies to the external binary
backend only — the embedded library is always `helm.sh/helm/v4` v4.2.4, well
past the 3.8.0 floor, so it always supports OCI unconditionally. OCI registry
support became generally available in helm 3.8.0. Earlier releases treat it
as experimental and refuse to act on an `oci://` reference unless
`HELM_EXPERIMENTAL_OCI=1` is set in the environment, which krmgen does not
set. Measured on helm 3.7.2:

```
Error: this feature has been marked as experimental and is not enabled by
default. Please set HELM_EXPERIMENTAL_OCI=1 in your environment to use this
feature
```

krmgen surfaces that error and exits 1. A configuration using only HTTP(S) chart
repositories may well work on helm releases older than 3.8.0 — every flag krmgen
passes predates it — but that combination is untested and unsupported.

**helm 2 is not supported** and will not be. It renders through a cluster-side
Tiller, which is a different architecture from the offline rendering this
document specifies; it reached end of life in November 2020.

**The kubectl floor applies only to the external backend.** When
`KRMGEN_KUBECTL_EXECUTABLE` is set, krmgen invokes `kubectl kustomize <dir>`, a
subcommand available since kubectl 1.14, and which kustomization features work
is decided by the Kustomize version that kubectl ships — see "Kustomize
version follows kubectl" below. Only the version in the table above has been
verified. Without that variable set, kustomize renders through the library
compiled into krmgen (Section 5), whose version is pinned in `go.mod` and does
not depend on the host's kubectl at all.

### Known differences between helm versions

**helm 4 writes OCI pull progress to stdout; krmgen strips it.** When a chart
is pulled from an OCI registry, helm 4 prints `Pulled: <ref>` and
`Digest: <sha256>` to **stdout**, ahead of the rendered manifests. helm 3 does
not print these lines. krmgen removes this: `binaryRenderer.Render`
(`internal/helm/renderer_binary.go:54`) passes helm's stdout through
`stripHelmBanner` (`internal/helm/renderer_binary.go:72`), which removes a
**leading contiguous run** of lines starting with `Pulled: `,
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

**Kustomize version follows kubectl — external backend only.** When
`KRMGEN_KUBECTL_EXECUTABLE` is set, the Kustomize version used is whatever the
installed kubectl ships. Two hosts with different kubectl versions can produce
different output from the same input on that path. This was the primary
motivation for adding the embedded backend (Section 5), which pins its
kustomize version in `go.mod` instead and is the default since this phase.

**`.Capabilities` is whatever the invoked helm defaults to.** krmgen never
passes `--kube-version` or `--api-versions` on the external path (see
Invocation, above), and the embedded renderer (`sdkRenderer.Render`,
`internal/helm/renderer_sdk.go`) never sets `action.Install.KubeVersion` /
`.APIVersions` either. `KubeVersion` is left at its zero value on both
backends. `.APIVersions` is not symmetric at the field level, though the end
result still is: helm's own CLI (`pkg/cmd/template.go`) defaults its
`--api-versions` flag to `[]string{}` and unconditionally assigns that to
`client.APIVersions`, an empty but **non-nil** slice — the embedded renderer
never touches the field, so it stays **nil**. This does not produce different
`.Capabilities`: `action.Install.RunWithContext` (`pkg/action/install.go:336`)
only does `i.cfg.Capabilities.APIVersions = append(i.cfg.Capabilities.APIVersions,
i.APIVersions...)`, and appending an empty slice or a nil slice is the same
no-op either way.
The design doc named this a risk for phase 6 (`docs/superpowers/specs/2026-08-20-krmgen-refaktoring-design.md`,
"Rizika a vědomé změny chování"): that an embedded library reimplementing
helm's CLI might quietly default `.Capabilities` differently from the real
`helm template` command. It does not: `pkg/cmd/template.go` and
`internal/helm/renderer_sdk.go` both construct an `action.Install` and call
its `RunWithContext` (`pkg/action/install.go`) — the *same* function, not a
reimplementation — so leaving `KubeVersion`/`APIVersions` unset (or set to an
empty slice, on the CLI side) produces identical `.Capabilities` defaults on
both backends whenever they run the same helm version, confirmed by reading
`helm.sh/helm/v4@v4.2.4`'s `pkg/cmd/template.go` and `pkg/action/install.go`
side by side. No golden fixture exercises this directly — none of the demo
charts read `.Capabilities` — so this is a code-reading confirmation, not a
test result.

The risk that remains, and always did, is across *different* helm versions: a
chart that branches on `.Capabilities.KubeVersion` or `.Capabilities.APIVersions`
still renders differently depending on which helm version is invoked, because
different helm versions ship different built-in defaults for a client-only
`template` run. An external binary older or newer than the pinned v4.2.4
embedded library is exactly such a case. This is not something krmgen
controls or records.

### Version detection

krmgen does not detect or report the version of the external tools it invokes
at runtime, and phase 6 did not add such a check — this remains open for a
later phase. What phase 6 added instead is a compile-time anchor, checked by
the test suite rather than surfaced to a user: `anchorHelmLibraryVersion` and
`TestEmbeddedHelmMatchesTheAnchor` (`test/golden/versions_test.go`) read the
`helm.sh/helm/v4` version actually linked into the built krmgen binary and
fail the test suite if it drifts from what the goldens were generated
against, mirroring `anchorKustomizeAPIVersion` /
`TestEmbeddedKustomizeMatchesTheAnchor` from phase 4. Both are safeguards for
whoever regenerates goldens, not something `krmgen generate` prints or a user
of a release binary ever sees. A user picking between backends in a bug
report still has no `krmgen`-reported version for either one — only the
docker image, the go.mod version this document names, or `helm version`/
`kubectl version` on their own host, if they know which backend they used.

## 6. Backend parity exceptions

krmgen has a second, embedded backend for both helm and kustomize, selected by
the absence of `KRMGEN_HELM_EXECUTABLE` and `KRMGEN_KUBECTL_EXECUTABLE`. Both
backends are measured against the same golden files. Where they cannot agree,
the difference is recorded here rather than treated as a defect:

| Capability | External binary | Embedded library |
|---|---|---|
| Helm plugins | Available | **Never available** |
| Helm post-renderers | Available | **Never available** |
| Kustomize version | Whatever kubectl ships | Pinned in `go.mod` |
| Helm version | Whatever the host has | Pinned in `go.mod` |
| OCI registry authentication | Writes credentials to disk, host-wide | In-memory, scoped to one render |
| Unserializable rendered resource | Error is redacted wholesale | Error names the offending resource |

Users depending on helm plugins or post-renderers must set
`KRMGEN_HELM_EXECUTABLE` and keep the external backend.

**A render error that would carry rendered content is redacted differently on
the two backends.** Neither prints kustomize's `%#v` dump of a resource it
could not serialize (see Error output, "Rendered resources are never echoed
into an error"), but only the embedded backend has the resource list in hand
and can therefore name the offending resource. Across the subprocess boundary
the external backend can only drop kubectl's stderr and point at the embedded
one. Exit code and the absence of rendered content are identical, and that is
what `TestError_RenderFailureDoesNotLeakSecretValues` asserts for both.

**OCI registry authentication uses a different mechanism on the two
backends, not just a different call.** When credentials are available for an
OCI chart, the external binary runs `helm registry login <registry-host>
--username <u> --password <p>` as a separate process before `helm template`
(`ociHelmGenerator.login`, `internal/helm/oci-generator.go`). This subprocess
writes the credentials into helm's on-disk registry config file
(`settings.RegistryConfig`, normally `~/.config/helm/registry/config.json` or
wherever `HELM_REGISTRY_CONFIG` points) and they persist there after krmgen
exits — any later `helm` invocation on that host, krmgen's or not, picks them
up. The embedded library instead hands the same credentials to
`registry.NewClient` via `registry.ClientOptBasicAuth(username, password)`
(`ociHelmGenerator.registryClient`, `internal/helm/oci-generator.go`, called
from `sdkRenderer.Render`, `internal/helm/renderer_sdk.go`), held only in
that one in-memory `*registry.Client` for the duration of that chart's
render; nothing is written to `settings.RegistryConfig`, which the embedded
path only ever reads, and only reaches at all in the no-credentials fallback
(e.g. `ignoreCredentials: true`, or an anonymous pull such as the
`oci-public` golden scenario). Net effect: a host that renders an OCI chart
with credentials via the external binary ends up with those credentials
cached in helm's config file for every later invocation on that host; the
same run via the embedded library leaves no such trace. This was verified by
reading `registry.NewClient`/`ClientOptBasicAuth`
(`helm.sh/helm/v4@v4.2.4/pkg/registry/client.go`) and `ociHelmGenerator.login`
side by side, not by a test — asserting "no file was written" would require
controlling `$HOME`/`HELM_REGISTRY_CONFIG` around a real `helm registry
login` subprocess, out of scope here. Rendered *output* is unaffected: both
backends render the `oci-public` scenario byte-identically
(`TestOci_BothHelmRenderersAgree`, `test/golden/oci_test.go`, behind the
`oci` build tag) — this is an authentication-mechanism difference only.

Both kustomize backends are required to render byte-identical output on
every scenario that renders successfully, and `TestGolden_BothBackendsAgree`
(`test/golden/harness_test.go`) enforces it for every such scenario the golden
suite covers: `kustomize-only`, `helm-with-kustomize`, `kustomize-features`.
Two scenarios that end in a kustomize error — `multi-config-kustomize` and
`nested-kustomization` — cannot be compared that way: stderr carries a
temporary directory path that differs between runs, and the external backend
wraps the underlying kustomize error in its own "run kubectl kustomize
failed" text where the embedded backend does not. For those,
`TestGolden_BothBackendsAgreeOnErrors` requires the same exit code and the
same stable substring in stderr instead of byte-identical stderr. Both
backends were found to raise the same underlying kustomize error in both
cases.

A third error scenario, `two-kustomizations`, is deliberately not part of
this comparison: `FindKustomizeFile` (`internal/kustomize/processor.go`)
fails on seeing multiple kustomization files before `BuildKustomize` ever
selects a backend, so that scenario never reaches either backend's code path
and says nothing about backend parity. `TestError_TwoKustomizations`
(`test/golden/errors_test.go`) covers it instead.

Where rendering succeeds on both, the one thing that differs between the
kustomize backends is the version: the external path renders with whatever
kustomize the installed kubectl embeds, which on an older host may be far
behind the pinned one. That is the trade the option exists to make. The helm
backends carry the same kind of version trade (Supported versions, Section 5)
plus the OCI authentication difference recorded above; where they render
successfully, output is otherwise byte-identical
(`TestGolden_BothHelmRenderersAgree`, `test/golden/harness_test.go`).

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
