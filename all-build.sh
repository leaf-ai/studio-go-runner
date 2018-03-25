#!/bin/bash -x
set -e
go get github.com/karlmutch/duat
go install github.com/karlmutch/duat/cmd/semver
go install github.com/karlmutch/duat/cmd/github-release
go install github.com/karlmutch/duat/cmd/image-release
go install github.com/karlmutch/duat/cmd/stencil
go run build.go -r
if [ $? -ne 0 ]; then
    echo "runner build failed"
    exit $?
fi

