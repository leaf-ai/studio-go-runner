#! /bin/bash
function create_all_branches()
{
    # Keep track of where Travis put us.
    # We are on a detached head, and we need to be able to go back to it.
    local build_head=$(git rev-parse HEAD)

    # Fetch all the remote branches. Travis clones with `--depth`, which
    # implies `--single-branch`, so we need to overwrite remote.origin.fetch to
    # do that.
    git config --replace-all remote.origin.fetch +refs/heads/*:refs/remotes/origin/*
    git fetch
    # optionally, we can also fetch the tags
    git fetch --tags

    # create the tacking branches
    for branch in $(git branch -r|grep -v HEAD) ; do
        git checkout -qf ${branch#origin/}
    done

    # finally, go back to where we were at the beginning
    git checkout ${build_head}
}

function fold_start()
{
  echo -e "travis_fold:start:$1\033[33;1m$2\033[0m"
}

function fold_end()
{
    echo -e "\ntravis_fold:end:$1\r"
}

create_all_branches
