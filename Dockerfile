FROM alpine:latest

ARG TARGETPLATFORM

LABEL maintainer="librucha@gmail.com"

# Switch to root for the ability to perform install
USER root

RUN apk add helm kubectl --no-cache

# install krmgen
COPY $TARGETPLATFORM/krmgen /bin/krmgen
RUN chmod +x /bin/krmgen

# create krmgen user
RUN delgroup $(cat /etc/group | grep 999 | cut -d: -f1)
RUN adduser -u 999 -D krmgen

# Switch back to non-root user
USER 999