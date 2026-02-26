FROM mcr.microsoft.com/azure-cli

ARG KRMGEN_VERSION
ARG TARGETPLATFORM

LABEL maintainer="librucha@gmail.com"
LABEL version=${KRMGEN_VERSION}

# Switch to root for the ability to perform install
USER root

RUN tdnf install helm kubectl --assumeyes

# install krmgen
COPY $TARGETPLATFORM/krmgen /bin/krmgen
RUN chmod +x /bin/krmgen

# Switch back to non-root user
USER nonroot
