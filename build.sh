#!/bin/bash -x

if ( find /project -maxdepth 0 -empty | read v );
then
  echo "source code must be mounted into the /project directory"
  exit 990
fi

export HASH=`git rev-parse HEAD`
export DATE=`date '+%Y-%m-%d_%H:%M:%S%z'`
export PATH=$PATH:$GOPATH/bin
go get -u github.com/golang/dep/cmd/dep
dep ensure -no-vendor
mkdir -p bin
go build -ldflags "-X main.buildTime=$DATE -X main.gitHash=$HASH" -o bin/runner cmd/runner/*.go
go build -ldflags "-X main.buildTime=$DATE -X main.gitHash=$HASH" -race -tags NO_CUDA -o bin/runner-cpu-race cmd/runner/*.go
go build -ldflags "-X main.buildTime=$DATE -X main.gitHash=$HASH" -tags NO_CUDA -o bin/runner-cpu cmd/runner/*.go
