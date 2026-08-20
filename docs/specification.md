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

To be completed in Task 3.

## 4. Template functions

To be completed in Task 4.

## 5. External tool support matrix

To be completed in Task 5.

## 6. Backend parity exceptions

To be completed in Task 6.

## 7. Non-goals

To be completed in Task 6.
