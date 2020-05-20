#!/bin/bash -e
set -x pipefail

[ -z "$USER" ] && echo "Error: env variable USER must be set" && exit 1;
[ -z "$GOPATH" ] && echo "Error: env variable GOPATH must be set" && exit 1;
[ -z "$GITHUB_TOKEN" ] && echo "Warning : env variable GITHUB_TOKEN should be set in the event that a release is to be generated" ;
[ -z ${azure_registry_name+x} ] && echo "Warning : env variable azure_registry_name not set";

if [[ ":$PATH:" != *":$GOPATH/bin:"* ]]; then
    export PATH=$PATH:$GOPATH/bin
fi

export LOGXI="*=DBG"
export LOGXI_FORMAT="happy,maxcol=1024"

[ -z "$TERM" ] && export TERM=xterm+256color;

if [ -n "$(type -t travis_fold)" ] && [ "$(type -t travis_fold)" = function ]; then
    type travis_fold
    type travis_nanoseconds
    type travis_time_start
    type travis_time_finish
:
else
declare -i travis_start_time
declare -i travis_end_time

	function travis_nanoseconds () {
		local cmd="date";
		local format="+%s%N";
		local os=$(uname);
		if hash gdate > /dev/null 2>&1; then
			cmd="gdate";
		else
			if [[ "$os" = Darwin ]]; then
				format="+%s000000000";
			fi;
		fi;
		$cmd -u $format
	}

    function travis_fold() {
        local action=$1;
        local name=$2;
        echo -en "travis_fold:${action}:${name}\r${ANSI_CLEAR}"
    }
    function travis_time_start() {
        travis_timer_id=$(printf %08x $(( RANDOM * RANDOM )));
        travis_start_time=$(travis_nanoseconds);
        echo -en "travis_time:start:$travis_timer_id\r${ANSI_CLEAR}"
    }
    function travis_time_finish() {
        local result=$?;
        travis_end_time=$(travis_nanoseconds);
        local duration=$(($travis_end_time-$travis_start_time));
        echo -en "\ntravis_time:end:$travis_timer_id:start=$travis_start_time,finish=$travis_end_time,duration=$duration\r${ANSI_CLEAR}";
        return $result

    }
fi

go get github.com/karlmutch/petname
go get github.com/ekalinin/github-markdown-toc.go
go get -u github.com/golang/dep/cmd/dep

dep ensure

# Get the documentation files with tables of contents
declare -a tocs=("README.md" "docs/azure.md" "docs/interface.md" "docs/ci.md" "docs/message_encryption.md" "examples/docker/README.md")

md_temp=$(mktemp -d)
for fn in "${tocs[@]}"
do
    github-markdown-toc.go $fn --hide-footer > $md_temp/header.md
    awk -v data="$(<$md_temp/header.md)" '
        BEGIN       {p=1}
        /^<!--ts-->/   {print;print data;p=0}
        /^<!--te-->/     {p=1}
        p' $fn > $md_temp/document.md
    cp $md_temp/document.md $fn
    rm $md_temp/document.md
    rm $md_temp/header.md
done


#go get -u github.com/gomarkdown/mdtohtml
#mdtohtml README.md $md_temp/README.html
#awk -v data="$(<$md_temp/README.html)" '/<!--bs-->/ {f=1} /<!--be-->/ && f {print data; f=0}1' docs/assets/README.tmpl > README.html
#rm $md_temp/README.html
rmdir $md_temp

bash -c "while true; do echo \$(date) - building ...; sleep 180s; done" &
PING_LOOP_PID=$!

function cleanup {
    # nicely terminate the ping output loop
    kill $PING_LOOP_PID
}
trap cleanup EXIT

function ExitWithError
{
    echo "$*">&2
    rm -f $working_file
    exit 1
}

function Tidyup
{
    ExitWithError "Abort"
}

umask 077
working_file=$$.studio-go-runner-working
rm -f $working_file
trap Tidyup 1 2 3 15

export SEMVER=`semver`
export GIT_BRANCH=`echo '{{.duat.gitBranch}}'|stencil -supress-warnings - | tr '_' '-' | tr '\/' '-'`
GIT_COMMIT=`git rev-parse HEAD`
export RUNNER_BUILD_LOG=build-$GIT_BRANCH.log
exit_code=0

export RepoVersion=`grep registry.version Dockerfile_base | cut -d= -f2 | cut -d\  -f1`
# See if the reference build base images exist
DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect quay.io/leafai/studio-go-runner-dev-base:$RepoVersion > /dev/null || true
exit_code=$?
if [ $exit_code -ne 0 ]; then
    # See if we have the base build image locally
    DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect leafai/studio-go-runner-dev-base:$RepoVersion > /dev/null
    if [ $exit_code -eq 0 ]; then
        docker tag leafai/studio-go-runner-dev-base:$RepoVersion quay.io/leafai/studio-go-runner-dev-base:$RepoVersion
        docker push quay.io/leafai/studio-go-runner-dev-base:$RepoVersion
    else
        # Build the base image that other images will derive from for development style images
        docker build -t studio-go-runner-dev-base:working -f Dockerfile_base .
        export RepoImage=`docker inspect studio-go-runner-dev-base:working --format '{{ index .Config.Labels "registry.repo" }}:{{ index .Config.Labels "registry.version"}}'`
        export RepoBaseImage=`docker inspect studio-go-runner-dev-base:working --format '{{ index .Config.Labels "registry.base" }}:{{ index .Config.Labels "registry.version"}}'`
        docker tag studio-go-runner-dev-base:working $RepoImage
        docker rmi studio-go-runner-dev-base:working

        docker tag $RepoImage quay.io/leafai/$RepoBaseImage
        docker push quay.io/leafai/$RepoBaseImage
    fi
fi

travis_fold start "build.image"
    travis_time_start
        # The workstation version uses the linux user ID of the builder to enable sharing of files between the
        # build container and the local file system of the user
        stencil -input Dockerfile_developer | docker build -t leafai/studio-go-runner-developer-build:$GIT_BRANCH -
        exit_code=$?
        if [ $exit_code -ne 0 ]; then
            exit $exit_code
        fi
		# Information about safely working with temporary files in shell scripts can be found at
        # https://dev.to/philgibbs/avoiding-temporary-files-in-shell-scripts
        {
            stencil -input Dockerfile_standalone > $working_file
            [[ $? != 0 ]] && ExitWithError "stencil processing of Dockerfile_standalone failed"
        } | tee $working_file > /dev/null
        [[ $? != 0 ]] && ExitWithError "Error writing to $working_file"
		docker build -t leafai/studio-go-runner-standalone-build:$GIT_BRANCH -f $working_file .
        rm -f $working_file
		docker tag leafai/studio-go-runner-standalone-build:$GIT_BRANCH leafai/studio-go-runner-standalone-build
		docker tag leafai/studio-go-runner-standalone-build:$GIT_BRANCH localhost:32000/leafai/studio-go-runner-standalone-build:latest
		docker tag leafai/studio-go-runner-standalone-build:$GIT_BRANCH localhost:32000/leafai/studio-go-runner-standalone-build:$GIT_BRANCH
        exit_code=$?
        if [ $exit_code -ne 0 ]; then
            exit $exit_code
        fi
    travis_time_finish
travis_fold end "build.image"

if [ $exit_code -ne 0 ]; then
    exit $exit_code
fi

# Running build.go inside of a container will result in a compilation, light testing, and release however no docker images
travis_fold start "build"
    travis_time_start
        container_name=`petname`
        # Dont release until after we check is microk8s is available for downstream testing
        docker run --name $container_name --user $(id -u):$(id -g) -e DEBUG="$DEBUG" -e TERM="$TERM" -e LOGXI="$LOGXI" -e LOGXI_FORMAT="$LOGXI_FORMAT" -v $GOPATH:/project leafai/studio-go-runner-developer-build:$GIT_BRANCH
        exit_code=`docker inspect $container_name --format='{{.State.ExitCode}}'`
        if [ $exit_code -ne 0 ]; then
            exit $exit_code
        fi
    travis_time_finish
travis_fold end "build"

if [ $exit_code -ne 0 ]; then
    exit $exit_code
fi

# Automatically produces images without compilation, or releases when run outside of a container
if docker image ls 2>/dev/null 1>/dev/null; then
    travis_fold start "image.build"
        travis_time_start
            cd cmd/runner && docker build -f Dockerfile.stock -t leafai/studio-go-runner:$SEMVER . ; cd ../..
            if az account list -otsv --all 2>/dev/null 1>/dev/null; then
                cd cmd/runner && docker build -f Dockerfile.azure -t leafai/azure-studio-go-runner:$SEMVER . ; cd ../..
            fi
            exit_code=$?
            if [ $exit_code -ne 0 ]; then
                exit $exit_code
            fi
        travis_time_finish
    travis_fold end "image.build"
fi

if [ $exit_code -ne 0 ]; then
    exit $exit_code
fi

# In the event that the following command was successful then we know a microk8s registry is present
# and we can defer any releases to the pipeline it is using rather than releasing from out
# current pipeline process
travis_fold start "image.ci_start"
    travis_time_start
        RegistryIP=`kubectl --namespace container-registry get pod --selector=app=registry -o jsonpath="{.items[*].status.hostIP}"||true`
        if [ $exit_code -eq 0 ]; then
            if [[ ! -z "$RegistryIP" ]]; then
                docker tag localhost:32000/leafai/studio-go-runner-standalone-build:$GIT_BRANCH \
                    $RegistryIP:32000/leafai/studio-go-runner-standalone-build:$GIT_BRANCH|| true
                docker push $RegistryIP:32000/leafai/studio-go-runner-standalone-build:$GIT_BRANCH || true
                docker tag localhost:32000/leafai/studio-go-runner-standalone-build:$GIT_BRANCH \
                    $RegistryIP:32000/leafai/studio-go-runner-standalone-build:latest|| true
                docker push $RegistryIP:32000/leafai/studio-go-runner-standalone-build:latest || true
                if [ $exit_code -eq 0 ]; then
                    exit $exit_code
                fi
            fi
        fi
    travis_time_finish
travis_fold end "image.ci_start"


travis_fold start "image.push"
    travis_time_start
        container_name=`petname`
        docker run --name $container_name --user $(id -u):$(id -g) -e "RELEASE_ONLY"="" -e DEBUG="$DEBUG" -e TERM="$TERM" -e LOGXI="$LOGXI" -e LOGXI_FORMAT="$LOGXI_FORMAT" -e GITHUB_TOKEN=$GITHUB_TOKEN -v $GOPATH:/project leafai/studio-go-runner-developer-build:$GIT_BRANCH
        exit_code=`docker inspect $container_name --format='{{.State.ExitCode}}'`
        if [ $exit_code -ne 0 ]; then
            exit $exit_code
        fi
		if docker image inspect leafai/studio-go-runner:$SEMVER 2>/dev/null 1>/dev/null; then
			if type docker 2>/dev/null ; then
                dockerLines=`docker system info 2>/dev/null | egrep "Registry: .*index.docker.io.*|User" | wc -l`
				if [ $dockerLines -eq 2 ]; then
                    docker push leafai/studio-go-runner:$SEMVER
                    docker push leafai/azure-studio-go-runner:$SEMVER
                fi
                docker tag leafai/studio-go-runner:$SEMVER quay.io/leafai/studio-go-runner:$SEMVER
                docker tag leafai/azure-studio-go-runner:$SEMVER quay.io/leafai/azure-studio-go-runner:$SEMVER

                # There is simply no reliable way to know if a docker login has been done unless, for example
                # config.json is not placed into your login directory, snap redirects etc so try and simply
                # silently fail.
                docker push quay.io/leafai/studio-go-runner:$SEMVER || true
                docker push quay.io/leafai/azure-studio-go-runner:$SEMVER || true
			fi
		fi
    travis_time_finish
travis_fold end "image.push"

exit 0
