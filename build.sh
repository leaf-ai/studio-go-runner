#!/bin/bash -e

[ -z "$USER" ] && echo "env variable USER must be set" && exit 1;
[ -z "$GITHUB_TOKEN" ] && echo "env variable GITHUB_TOKEN must be set as this is a proprietary repository for Sentient" && exit 1;
[ -z "$GOPATH" ] && echo "env variable GOPATH must be set" && exit 1;

if [[ ":$PATH:" != *":$GOPATH/bin:"* ]]; then
    export PATH=$PATH:$GOPATH/bin
fi

go get -u github.com/golang/dep/cmd/dep
go get -u -f github.com/karlmutch/duat/cmd/semver
go get -u -f github.com/karlmutch/duat/cmd/stencil

dep ensure

stencil -input Dockerfile | docker build -t runner-build --build-arg USER=$USER --build-arg USER_ID=`id -u $USER` --build-arg USER_GROUP_ID=`id -g $USER` -
# Running build.go inside of a container will result is a simple compilation and no docker images
docker run -e GITHUB_TOKEN=$GITHUB_TOKEN -v $GOPATH:/project runner-build
if [ $? -ne 0 ]; then
    echo ""
    exit $?
fi

# Automatically produces images, and github releases without compilation when run outside of a container
go run ./build.go -r cmd

export SEMVER=`semver`
if docker image inspect sentient-technologies/studio-go-runner/runner:$SEMVER 2>/dev/null 1>/dev/null; then
    if type aws 2>/dev/null ; then
        `aws ecr get-login --no-include-email --region us-west-2`
        if [ $? -eq 0 ]; then
            account=`aws sts get-caller-identity --output text --query Account`
            if [ $? -eq 0 ]; then
                docker tag sentient-technologies/studio-go-runner/runner:$SEMVER $account.dkr.ecr.us-west-2.amazonaws.com/sentient-technologies/studio-go-runner/runner:$SEMVER
                docker push $account.dkr.ecr.us-west-2.amazonaws.com/sentient-technologies/studio-go-runner/runner:$SEMVER
            fi
        fi
    fi
    if type az 2>/dev/null; then
        if az acr login --name sentientai; then
            docker tag sentient-technologies/studio-go-runner/runner:$SEMVER sentientai.azurecr.io/sentient-technologies/studio-go-runner/runner:$SEMVER
            docker push sentient-technologies/studio-go-runner/runner:$SEMVER
        fi
    fi
fi
