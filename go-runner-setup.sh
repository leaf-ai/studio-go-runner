#!/bin/bash

# Usage:
# cmd/unileaf-go-setup.sh [mdserver|mdserver-gateway]
#
# This script is meant to setup the go environment and
# build the mdserver and/or the mdserver-gateway.
#
# Additional parameters supplied as env var are needed as well.
# Required:
#   GOROOT - where your golang tool root lives
#   GOPATH - the root for your golang source (see the cmd/mdserver.adoc for deets)
#
# Optional:
#   APP_HOME -          Where the unileaf source is git cloned into.
#                       If not set we assume you are at the top-level unileaf directory
#   COMPONENT_NAME -    The target of the build
#                       If not set, we default to building mdserver only.
#                       Will be overridden by the first command line argument.
#   PROTOBUF_VERSION -  The version of the protoc compiler to use.
#                       If not set, we default to a reasonable version used in builds.
#   PROTOBUF_ZIP -      The zip file containing the protoc compiler to use.
#                       If not set, we default to a reasonable version used in builds.
#   PROTOBUF_URL -      The url where to find the zip file containing the protoc compiler to use.
#                       If not set, we default to a reasonable version used in builds.
#
# See build_scripts/go_codefresh.yml and cmd/Dockerfile for
# example of usage within our CI/CD system.

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

export COMPONENT=runner-linux-amd64-cpu

go clean -i
go mod tidy
go build -ldflags="-extldflags=-static" -tags="NO_CUDA" -o cmd/runner/bin/"${COMPONENT_NAME}"  ./cmd/runner

echo "ALL DONE building ${COMPONENT_NAME}"
