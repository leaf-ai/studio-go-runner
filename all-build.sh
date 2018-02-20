#!/bin/bash -x
set -e
go install github.com/karlmutch/bump-ver/cmd/bump-ver
./cmd/runner/build.sh
if [ $? -ne 0 ]; then
    echo "runner build failed"
    exit $?
fi

