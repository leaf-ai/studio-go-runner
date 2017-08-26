#!/bin/bash -x

if ( find /project -maxdepth 0 -empty | read v );
then
  echo "source code must be mounted into the /project directory"
  exit 990
fi

export PATH=$PATH:$GOPATH/bin
go get -u github.com/golang/dep/cmd/dep
dep ensure -no-vendor
go build cmd/runner/*.go
