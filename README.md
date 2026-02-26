# krmgen

[![Build](https://github.com/librucha/krmgen/actions/workflows/go.yml/badge.svg)](https://github.com/librucha/krmgen/actions/workflows/go.yml)

cli tool for generate Kubernetes Resource Model from helm and kustomization together. it uses Kubernetes like configuration for inputs definition

## Usage Guide

Create minimal krmgen config file for helm template:
```bash
cat <<EOF > krmgen.yaml
apiVersion: krmgen.config.librucha.com/v1alpha1
kind: KrmGen

helm:
  charts:
    - name: hello-world
      repo: https://helm.github.io/examples
      releaseName: krmgen-example
EOF
```

create minimal kustomize config (optional):
```bash
cat <<EOF > kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: default

labels:
  - pairs:
      app.kubernetes.io/generated-by: '{{ krmgenGenerated }}'
EOF
```

run generator
```bash
krmgen generate .
```

---

See the [documentation](https://librucha.gitbook.io/krmgen) for more examples.

## Install

### [Download the latest binary](https://github.com/librucha/krmgen/releases/latest)

### wget
Use wget to download, gzipped pre-compiled binaries:


For instance, VERSION=v1.0.0 and BINARY=krmgen_linux_amd64

#### Compressed via tar.gz
```bash
VERSION=v1.0.0
BINARY=krmgen_linux_amd64

wget https://github.com/librucha/krmgen/releases/download/${VERSION}/${BINARY}.tar.gz -O - |\
  tar xz && mv ${BINARY} /usr/bin/krmgen
```

#### Plain binary

```bash
VERSION=v1.0.0
BINARY=krmgen_linux_amd64

wget https://github.com/librucha/krmgen/releases/download/${VERSION}/${BINARY} -O /usr/bin/krmgen &&\
    chmod +x /usr/bin/krmgen
```

#### Latest version

```bash
wget https://github.com/librucha/krmgen/releases/latest/download/krmgen_linux_amd64 -O /usr/bin/krmgen &&\
    chmod +x /usr/bin/krmgen
```

### generate using docker image

```bash
docker run librucha/krmgen:latest krmgen --version
docker run -v /tmp:/tmp -v ./:/home/krmgen/example librucha/krmgen:latest krmgen generate /home/krmgen/example
```