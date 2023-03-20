#!/bin/bash

# Original file from Mike Farah https://github.com/mikefarah/yq/blob/master/scripts/test.sh

set -o errexit
set -o nounset
set -o pipefail

export CGO_ENABLED=0

XDG_CACHE_HOME=/tmp/ go test $(go list ./... )
echo "Success!"