#!/bin/bash -e

# Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

if ( find /project -maxdepth 0 -empty | read v );
then
  echo "source code must be mounted into the /project directory"
  exit 990
fi

export HASH=`git rev-parse HEAD`
export DATE=`date '+%Y-%m-%d_%H:%M:%S%z'`
export PATH=$PATH:$GOPATH/bin
go get -u -f github.com/aktau/github-release
go get -u -f github.com/karlmutch/bump-ver/cmd/bump-ver

export SEMVER=`bump-ver extract`
TAG_PARTS=$(echo $SEMVER | sed "s/-/\n-/g" | sed "s/\./\n\./g" | sed "s/+/\n+/g")
PATCH=""
for part in $TAG_PARTS
do
    start=`echo "$part" | cut -c1-1`
    if [ "$start" == "+" ]; then
        break
    fi
    if [ "$start" == "-" ]; then
        PATCH+=$part
    fi
done

flags='-X main.buildTime="$DATE" -X main.gitHash="$HASH" -X command-line-arguments.TestRunMain=Use -X command-line-arguments.buildTime="$DATE" -X command-line-arguments.gitHash="$HASH"'

mkdir -p cmd/runner/bin
go build -ldflags "$flags" -o cmd/runner/bin/runner cmd/runner/*.go
go build -ldflags "$flags" -race -tags NO_CUDA -o cmd/runner/bin/runner-cpu-race cmd/runner/*.go
go build -ldflags "$flags" -tags NO_CUDA -o cmd/runner/bin/runner-cpu cmd/runner/*.go
go test -ldflags "$flags" -coverpkg="." -c -o cmd/runner/bin/runner-cpu-run-coverage -tags 'NO_CUDA' cmd/runner/*.go
go test -ldflags "$flags" -coverpkg="." -c -o cmd/runner/bin/runner-cpu-test-coverage -tags 'NO_CUDA' cmd/runner/*.go
go test -ldflags "$flags" -race -c -o cmd/runner/bin/runner-cpu-test -tags 'NO_CUDA' cmd/runner/*.go
if [ -z "$PATCH" ]; then
    if ! [ -z "${SEMVER}" ]; then
        if ! [ -z "${GITHUB_TOKEN}" ]; then
            github-release release --user karlmutch --repo studio-go-runner --tag ${SEMVER} --pre-release && \
            github-release upload --user karlmutch --repo studio-go-runner  --tag ${SEMVER} --name runner --file bin/runner
        fi
    fi
fi
