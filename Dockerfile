FROM alpine:latest

ARG TARGETPLATFORM

LABEL maintainer="librucha@gmail.com"

# Switch to root for the ability to perform install
USER root

# helm and kubectl are intentionally NOT installed here: krmgen renders both
# Helm charts and Kustomize overlays through embedded libraries by default.
# The external binaries are only used as an opt-in (KRMGEN_HELM_EXECUTABLE /
# KRMGEN_KUBECTL_EXECUTABLE) — whoever needs that backend installs it into
# their own image. git stays because chart/config sources fetched by the
# surrounding tooling may need it.
RUN apk add git --no-cache

# install krmgen
COPY $TARGETPLATFORM/krmgen /bin/krmgen
RUN chmod +x /bin/krmgen

# create krmgen user
RUN delgroup $(cat /etc/group | grep 999 | cut -d: -f1)
RUN adduser -u 999 -D krmgen

# Switch back to non-root user
USER 999