set -x
export PATH=$PATH:$GOPATH/bin
go get -u github.com/golang/dep/cmd/dep
dep ensure
go build -v cmd/runner/*.go
