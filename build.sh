#!/bin/bash -e

[ -z "$USER" ] && echo "env variable USER must be set" && exit 1;
[ -z "$GITHUB_TOKEN" ] && echo "env variable GITHUB_TOKEN must be set as this is a proprietary repository for Sentient" && exit 1;
[ -z "$GOPATH" ] && echo "env variable GOPATH must be set" && exit 1;

if [[ ":$PATH:" != *":$GOPATH/bin:"* ]]; then
    export PATH=$PATH:$GOPATH/bin
fi

bump-ver -t ./Dockerfile -f ./README.md inject | docker build -t runner-build --build-arg USER=$USER --build-arg USER_ID=`id -u $USER` --build-arg USER_GROUP_ID=`id -g $USER` -
go get -u github.com/golang/dep/cmd/dep
go get -u github.com/karlmutch/bump-ver/cmd/bump-ver
dep ensure
docker run -e GITHUB_TOKEN=$GITHUB_TOKEN -v $GOPATH:/project runner-build
if [ $? -ne 0 ]; then
    echo ""
    exit $?
fi

cd cmd/runner
. deploy.sh
cd ../..

export SEMVER=`bump-ver extract`
if docker image inspect runner:$SEMVER 2>/dev/null 1>/dev/null; then
    if type aws 2>/dev/null ; then
        `aws ecr get-login --no-include-email --region us-west-2`
        if [ $? -eq 0 ]; then
            account=`aws sts get-caller-identity --output text --query Account`
            if [ $? -eq 0 ]; then
                docker tag sentient.ai/studio-go-runner:$SEMVER $account.dkr.ecr.us-west-2.amazonaws.com/sentient.ai/studio-go-runner:$SEMVER
                docker push $account.dkr.ecr.us-west-2.amazonaws.com/sentient.ai/studio-go-runner:$SEMVER
            fi
        fi
    fi
fi

if type az 2>/dev/null; then
    if az acr login --name sentientai; then
        docker tag sentient.ai/studio-go-runner:$SEMVER sentientai.azurecr.io/sentient.ai/studio-go-runner:$SEMVER
        docker push sentientai.azurecr.io/sentient.ai/studio-go-runner:$SEMVER
    fi
fi
