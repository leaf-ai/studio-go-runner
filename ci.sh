#!/bin/bash -e

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

go get -u github.com/golang/dep/cmd/dep

dep ensure

wget -O $GOPATH/bin/semver https://github.com/karlmutch/duat/releases/download/0.9.2/semver-linux-amd64
wget -O $GOPATH/bin/stencil https://github.com/karlmutch/duat/releases/download/0.9.2/stencil-linux-amd64
chmod +x $GOPATH/bin/semver
chmod +x $GOPATH/bin/stencil

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
export GIT_BRANCH=`echo '{{.duat.gitBranch}}'|stencil - | tr '_' '-' | tr '\/' '-'`
GIT_COMMIT=`git rev-parse HEAD`
export RUNNER_BUILD_LOG=build-$GIT_BRANCH.log
exit_code=0

# Determine if we are running under a keel based CI build and if so ...
export
if [ -z ${KeelCI+x} ]; then
    echo "Look for deployments to scale to 0 to lighten the load and footprint from this process"
    kubectl --namespace $K8S_NAMESPACE get deployments
    kubectl --namespace $K8S_NAMESPACE -o go-template --template="{{range .items}}kubectl scale --namespace {{.metadata.namespace}} --replicas=0 rc/{{.metadata.name}}{{end}}" get rc
fi

travis_fold start "build.image"
    travis_time_start
        set -o pipefail ; (go run build.go -r -dirs=internal && go run build.go -r -dirs=cmd && echo "Success" || echo "Failure") 2>&1
        exit_code=$?
        if [ $exit_code -ne 0 ]; then
            exit $exit_code
        fi
    travis_time_finish
travis_fold end "build.image"

if [ $exit_code -ne 0 ]; then
    exit $exit_code
fi

exit 0
