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
#   GIT_BRANCH -        Current git branch for code being built
#   GIT_COMMIT -        Current git commit id for code being built

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

if [ -z "${GIT_BRANCH}" ]
then
    export GIT_BRANCH="unknown branch"
fi
if [ -z "${GIT_COMMIT}" ]
then
    export GIT_COMMIT="unknown commit"
fi

source generate_git_info.sh "${GIT_BRANCH}" "${GIT_COMMIT}"

export COMPONENT_NAME=runner-linux-amd64-cpu

go clean -i
go clean -modcache
go clean -cache
go clean -testcache
go mod tidy
go build -ldflags="-extldflags=-static" -buildvcs=false -tags="NO_CUDA" -o cmd/runner/bin/"${COMPONENT_NAME}"  ./cmd/runner

echo "ALL DONE building ${COMPONENT_NAME}"
