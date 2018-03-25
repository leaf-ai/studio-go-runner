#!/bin/bash

[ -z "$USER" ] && echo "env variable USER must be set" && exit 1;

go get -u -f github.com/karlmutch/duat/cmd/semver

export SEMVER=`semver -f ../../README.md extract`

semver -git ../.. -t Dockerfile -f ../../README.md inject > Dockerfile.tmp
docker build -t sentient.ai/studio-go-runner:${SEMVER} -f Dockerfile.tmp .
