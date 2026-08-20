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

1. Read `skip` patterns from every `kind: KrmGen` file in the source directory.
   This happens on the **raw YAML**, before any template evaluation — the patterns
   themselves cannot be templated.
2. Merge config-level `skip` patterns with `--skip` flags. Order is preserved,
   duplicates removed, config patterns first.
3. Copy the source directory to a temporary working directory. Every file is
   evaluated as a Go template **except** files matching a skip pattern, which are
   copied byte-for-byte.
4. For each `kind: KrmGen` file in the working directory: run helm for every
   declared chart, concatenating the output in declaration order.
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

To be completed in Task 4.

## 5. External tool support matrix

To be completed in Task 5.

## 6. Backend parity exceptions

To be completed in Task 6.

## 7. Non-goals

To be completed in Task 6.
