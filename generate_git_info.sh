#!/bin/bash

# Get the current git commit ID
# git_commit=$(git rev-parse HEAD)
git_commit=$1

# Get the current git branch name
# git_branch=$(git rev-parse --abbrev-ref HEAD)
git_branch=$2

# Define the output file
output_file="cmd/runner/git_info.go"

rm -rf ${output_file}

# Generate the Go source file
cat <<EOL > ${output_file}
package main

// GitCommit is the current git commit ID
var gitCommit string = "${git_commit}"

// GitBranch is the current git branch name
var gitBranch string = "${git_branch}"
EOL

echo "Generated ${output_file} with current git commit ID and branch name."

