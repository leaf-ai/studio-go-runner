# studio-go-runner
Repository containing a TensorFlow Studio runner as an entirely decoupled implementation of a runner for the Sentient deployments of Studio.

This tool is intended to be used as a statically compiled version of the python runner using Go from Google.  It is intended to be run as a proof of concept for validating that:

1. Work within TFStudio can be routed from a queuing infrastructure to a scheduling infrastructure typical of Datacenters and inhouse compute resources.
2. If containers can be deployed using Bare metal tools such as Singularity are also a possibility.
3. If containers using purely AWS MetaData and S3 access can be deployed within TFStudio.
4. Accessing PubSub is viable for heterogenous OS's and hardware such as ARM64 could be used, not a specific TFStudio test but more general.

# Using the code

The github repository should be cloned an existing git clone of the https://github.com/ilblackdragon/studio.git repo.  Within the studio directories create a sub directory src and set your GOPATH to point at the top level studio directory.

    git clone https://github.com/ilblackdragon/studio.git
    cd studio
    export GOPATH=`pwd`
    mkdir src
    cd src
    git clone https://github.com/karlmutch/studio-go-runner.git
    go run cmd/runner/main.go

# Go compilation

This code based makes use of Go 1.9 release candidates at the time it was authored. The code will cleanly compile with Go 1.9 when it is released with 4 weeks.  In the interim however the release candiates should be used for building this code.
Instructions for Go 1.9 release candiate installation can be found at https://godoc.org/golang.org/x/build/version/go1.9rc1.
go dep is used as the dependency management tool.  You do not need to use this tool except during active development. go dep can be found at https://github.com/golang/dep.  go dep is intended to be absorbed into the go toolchain but for now can be obtained independently if needed.  All dependencies for this code base are checked into github following the best practice suggested at https://www.youtube.com/watch?v=eZwR8qr2BfI.
