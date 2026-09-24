<!--
  Docker Hub overview for librucha/krmgen, pushed by the GoReleaser Pro
  `dockerhub` pipe (.goreleaser.yaml) on every tagged release.

  Keep it free of Go template delimiters (double curly braces): GoReleaser
  may evaluate this file as a template, and krmgen's own template syntax
  would break the release. Link to the GitHub README for template examples.
  Docker Hub caps the full description at 25,000 characters.
-->

# krmgen

Render **Helm charts and Kustomize overlays in one pass** into plain, static
Kubernetes YAML — with Go templates evaluated in every input file first, so
secrets and environment-specific values land in the manifests at generation
time, with no cluster-side tooling.

Built for GitOps pipelines (Argo CD Config Management Plugins, Flux, CI jobs)
that need fully rendered YAML on stdout.

- **Source & full documentation:** https://github.com/librucha/krmgen
- **Releases & changelog:** https://github.com/librucha/krmgen/releases
- **Issues:** https://github.com/librucha/krmgen/issues

## Tags

| Tag | Meaning |
|---|---|
| `latest` | Most recent release |
| `<version>` (e.g. `1.0.2`) | Immutable release, no `v` prefix — pin this in production |

Every tag is a multi-arch manifest for `linux/amd64` and `linux/arm64`.

## What's inside

| | |
|---|---|
| Base | `alpine:latest` |
| Binary | `/bin/krmgen` (static, `CGO_ENABLED=0`) |
| Extra packages | `git` |
| User | `krmgen`, UID/GID `999` (non-root) |
| Entrypoint | none — pass `krmgen ...` as the command |

`helm` and `kubectl` are **not** installed and not needed: krmgen renders
charts and kustomizations through the Helm and Kustomize libraries compiled
into the binary. To use external binaries instead, build your own image on
top of this one and set `KRMGEN_HELM_EXECUTABLE` / `KRMGEN_KUBECTL_EXECUTABLE`.

## Usage

```bash
# Version
docker run --rm librucha/krmgen:latest krmgen --version

# Render the current directory
docker run --rm -v "$PWD:/workspace" -w /workspace \
  librucha/krmgen:latest krmgen generate . > manifests.yaml

# Skip template evaluation for binary files
docker run --rm -v "$PWD:/workspace" -w /workspace \
  librucha/krmgen:latest krmgen generate . --skip='*.pfx' --skip='*.png'
```

The container runs as UID 999, so the mounted directory must be readable by
that UID. krmgen only reads the source directory; its working copy goes to
the container's temp directory and is removed when the run ends.

### Azure Key Vault / Storage / Managed Identity

Template functions authenticate with the Azure SDK's `DefaultAzureCredential`,
so the standard variables work:

```bash
docker run --rm -v "$PWD:/workspace" -w /workspace \
  -e AZURE_TENANT_ID -e AZURE_CLIENT_ID -e AZURE_CLIENT_SECRET \
  librucha/krmgen:latest krmgen generate .
```

On AKS, Workload Identity works without any of these variables.

### Private Helm repositories

```bash
docker run --rm -v "$PWD:/workspace" -w /workspace \
  -e KRMGEN_HELM_USERNAME -e KRMGEN_HELM_PASSWORD \
  librucha/krmgen:latest krmgen generate .
```

Used as a fallback when `repoUser` / `repoPassword` are not set in
`krmgen.yaml`.

### Argo CD Config Management Plugin (sidecar)

```yaml
# argocd-repo-server Deployment patch
spec:
  template:
    spec:
      containers:
        - name: krmgen
          image: librucha/krmgen:latest   # pin a release tag in production
          command: [/var/run/argocd/argocd-cmp-server]
          securityContext:
            runAsNonRoot: true
            runAsUser: 999
          volumeMounts:
            - { name: var-files, mountPath: /var/run/argocd }
            - { name: plugins, mountPath: /home/argocd/cmp-server/plugins }
            - { name: krmgen-plugin, mountPath: /home/argocd/cmp-server/config/plugin.yaml, subPath: plugin.yaml }
            - { name: cmp-tmp, mountPath: /tmp }
      volumes:
        - { name: krmgen-plugin, configMap: { name: krmgen-plugin } }
        - { name: cmp-tmp, emptyDir: {} }
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: krmgen-plugin
data:
  plugin.yaml: |
    apiVersion: argoproj.io/v1alpha1
    kind: ConfigManagementPlugin
    metadata:
      name: krmgen
    spec:
      generate:
        command: [krmgen, generate, .]
      discover:
        fileName: krmgen.yaml
```

Argo CD runs the command in the application's source directory; Application
`plugin.env` entries reach templates through the `argocdEnv` function
(`ARGOCD_ENV_<key>` / `ARGOCD_APP_<key>`).

## Minimal config

```yaml
# krmgen.yaml
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

Add a `kustomization.yaml` next to it and the rendered chart is injected as a
resource and run through Kustomize. Template functions (Azure Key Vault,
`argocdEnv`, `kubeEnv`, `readF`, all of sprig except `env` / `expandenv`) and
the full configuration reference are documented in the
[README](https://github.com/librucha/krmgen#template-functions).

## Environment variables

| Variable | Purpose |
|---|---|
| `KRMGEN_HELM_USERNAME` / `KRMGEN_HELM_PASSWORD` | Helm repo credentials fallback |
| `KRMGEN_HELM_EXECUTABLE` | Use an external `helm` binary instead of the embedded library |
| `KRMGEN_KUBECTL_EXECUTABLE` | Use external `kubectl kustomize` instead of the embedded library |
| `AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, … | Azure SDK authentication |
