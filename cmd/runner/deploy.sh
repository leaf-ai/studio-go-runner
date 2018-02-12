#!/bin/bash

[ -z "$USER" ] && echo "env variable USER must be set" && exit 1;

go get -u -f github.com/karlmutch/bump-ver/cmd/bump-ver

export SEMVER=`bump-ver -f ../../README.md extract`
TAG_PARTS=$(echo $SEMVER | sed "s/-/\n-/g" | sed "s/\./\n\./g" | sed "s/+/\n+/g")
PATCH=""
for part in $TAG_PARTS
do
    if [ "$part" = "" ]; then
        break
    fi
    start=`echo "$part" | cut -c1-1`
    if [ "$start" = "+" ]; then
        break
    fi
    if [ "$start" = "-" ]; then
        PATCH+=$part
    fi
done

bump-ver -git ../.. -t Dockerfile -f ../../README.md inject > Dockerfile.tmp
docker build -t sentient.ai/studio-go-runner:${SEMVER} -f Dockerfile.tmp .
