#!/bin/bash
set -x

if ( find /project -maxdepth 0 -empty | read v );
then
  echo "Error: Must mount Go source code into /project directory"
  exit 990
fi

export PATH=$PATH:$GOPATH/bin
go get -u github.com/golang/dep/cmd/dep
dep ensure
go build -v cmd/runner/*.go
