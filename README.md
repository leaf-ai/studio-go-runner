# studio-go-runner
Repository containing a TensorFlow studioml runner as an entirely decoupled implementation of a runner for the Sentient deployments of studioml.

This tool is intended to be used as a statically compiled version of the python runner using Go from Google.  It is intended to be run as a proof of concept for validating that:

1. Work within studioml can be routed from a queuing infrastructure to a scheduling infrastructure typical of Datacenters and inhouse compute resources.
2. If containers can be deployed using Bare metal tools such as Singularity are also a possibility.
3. If containers using purely AWS MetaData and S3 access can be deployed within studioml.
4. Accessing PubSub is viable for heterogenous OS's and hardware such as ARM64 could be used, not a specific studioml test but more general.

# Using the code

The github repository should be cloned an existing git clone of the https://github.com/studioml/studio.git repo.  Within the studio directories create a sub directory src and set your GOPATH to point at the top level studio directory.

    git clone https://github.com/studioml/studio.git
    cd studio
    export GOPATH=`pwd`
    export PATH=~/studio/bin:$PATH
    mkdir -p src/github.com/SentientTechnologies
    cd src/github.com/SentientTechnologies
    git clone https://github.com/SentientTechnologies/studio-go-runner.git
    cd studio-go-runner
    go run cmd/runner/main.go

# Go compilation

This code based makes use of Go 1.9 release candidates at the time it was authored. The code will cleanly compile with Go 1.9 when it is released with 4 weeks.  In the interim however the release candiates should be used for building this code.
Instructions for Go 1.9 release candiate installation can be found at https://godoc.org/golang.org/x/build/version/go1.9rc1.
go dep is used as the dependency management tool.  You do not need to use this tool except during active development. go dep software, and its installation instructions can be found at https://github.com/golang/dep.  go dep is intended to be absorbed into the go toolchain but for now can be obtained independently if needed.  All dependencies for this code base are checked into github following the best practice suggested at https://www.youtube.com/watch?v=eZwR8qr2BfI.

# Runtime Environment
studioml uses the python virtual environment tools to deploy python applications and uses no isolation other than that offered by python.

The go based runner can make use of Singularity, a container platform, to provide isolation and also access to low level machine resources such as GPU cards.  This fuctionality is what differentiates the go based runner from the python based runners that are found within the open source studioml offering.  Singlularity support is offered as an extension to the studioml ecosystem however using its use while visible to studioml affects it in no way.

# Data storage support

The go runner support both S3 and Google Cloud storage models.

The google storage model allows for google cloud data to be used with the go runner being used in a private mode with a singlew set of credentials.  The environment variables GOOGLE_APPLICATION_CREDENTIALS, and GOOGLE_FIREBASE_CREDENTIALS being set to respective files for credential information.

A yaml configuration file for google storage can be specified like the following:

```
database:
    type: FireBase

    apiKey: **REDACTED**
    projectId: tfstudio-a8367
    messagingSenderId: 99999999999

    authDomain: "{}.firebaseapp.com"
    databaseURL: "https://{}.firebaseio.com"
    storageBucket: "{}.appspot.com"


    use_email_auth: true

storage:
    type: gcloud
    bucket: "tfstudio-a8367.appspot.com"

saveWorkspaceFrequency: 1 #how often is workspace being saved (minutes)
verbose: error

cloud:
    type: none
```

The S3 storage support can be used for runners that are either shared or are privately configured.  When using studioml to submit work the experimenter can used the yaml configuration file to pass their local AWS configuration environment variables through to the runner using a file such as the following:

```
database:
    type: FireBase

    apiKey: **REDACTED**
    projectId: tfstudio-a8367
    messagingSenderId: 99999999999

    authDomain: "{}.firebaseapp.com"
    databaseURL: "https://{}.firebaseio.com"
    storageBucket: "{}.appspot.com"


    use_email_auth: true

storage:
    type: s3
    endpoint: s3-us-west-2.amazonaws.com
    bucket: "karl-mutch"

saveWorkspaceFrequency: 1 #how often is workspace being saved (minutes)
verbose: error

cloud:
    type: none

env:
    AWS_ACCESS_KEY_ID: $AWS_ACCESS_KEY_ID
    AWS_DEFAULT_REGION: $AWS_DEFAULT_REGION
    AWS_SECRET_ACCESS_KEY: $AWS_SECRET_ACCESS_KEY

```

The above is an example of using google PubSub to pass messages while using the public AWS S3 service as the primary storage.

If a local deployment of an S3 compatible service is being used then the endpoint entry for the storage section can point at your local host, for example a minio.io server.
