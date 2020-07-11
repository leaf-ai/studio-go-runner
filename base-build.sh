#!/bin/bash

set -e
set -o pipefail
go get -u google.golang.org/protobuf/cmd/protoc-gen-go
go install google.golang.org/protobuf/cmd/protoc-gen-go
[ -e internal/gen ] || mkdir -p internal/gen
protoc -Iproto -I/usr/include --plugin=$GOPATH/bin/protoc-gen-go --go_out=./internal/gen proto/reports.proto
(dep ensure && go run build.go -r -dirs=internal && go run build.go -r -dirs=cmd) 2>&1 | tee "$RUNNER_BUILD_LOG"
