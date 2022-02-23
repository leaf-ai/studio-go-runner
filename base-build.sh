#!/bin/bash

set -e
set -o pipefail
go get -u google.golang.org/protobuf/cmd/protoc-gen-go
go mod vendor
[ -e internal/gen ] || mkdir -p internal/gen
protoc -Iproto -I/usr/include --plugin=$GOPATH/bin/protoc-gen-go --go_out=./internal/gen --python_out=./assets/response_catcher proto/reports.proto
protoc -Iproto -I/usr/include --plugin=$GOPATH/bin/protoc-gen-go --go_out=./internal/gen proto/tensorflow_serving/config/*.proto
GITMARK=cmd/runner/gitmarker.go
rm -f $GITMARK
source gitmarker.sh
(go run build.go -r -dirs=tools/serving-bridge,tools/queue-scaler,internal,cmd) 2>&1 | tee "$RUNNER_BUILD_LOG"
