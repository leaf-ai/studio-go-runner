#!/bin/bash

# Usage:
# ./go-runner-setup.sh
#
# This script is meant to setup the go environment and
# build the go-runner.
#
# Additional parameters supplied as env var are needed as well.
# Required:
#   GOROOT - where your golang tool root lives
#
# Optional:
#   APP_HOME -          Where the go-runner source is git cloned into.
#                       If not set we assume you are at the top-level unileaf directory

set -o errexit
set -o errtrace
set -o pipefail
set -o xtrace

# We expect APP_HOME to be set in a container setting,
# not so much in a development environment.
if [ -n "${APP_HOME}" ]
then
    cd "${APP_HOME}"
fi

export COMPONENT_NAME=runner-linux-amd64-cpu

go clean -i
go mod tidy
go build -ldflags="-extldflags=-static" -buildvcs=false -tags="NO_CUDA" -o cmd/runner/bin/"${COMPONENT_NAME}"  ./cmd/runner

echo "ALL DONE building ${COMPONENT_NAME}"
