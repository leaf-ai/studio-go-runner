#!/bin/bash
set -e

GIT_COMMIT=`git rev-parse HEAD`
GIT_BRANCH=`git branch | grep \* | cut -d " " -f2`
TARGET=./cmd/runner/gitmarker.go

echo "// Copyright 2018-2022 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License." > $TARGET
echo "" >>$TARGET
echo "package main" >>$TARGET
echo "" >>$TARGET
echo "var (" >>$TARGET
echo "      gitBranch = \"$GIT_BRANCH\"" >>$TARGET
echo "      gitCommit = \"$GIT_COMMIT\"" >>$TARGET
echo ")" >>$TARGET
echo "" >>$TARGET


