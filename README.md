# studio-go-runner

studio-go-runner is an implementation of a runner for the Sentient deployments of studioml.

The primary role of studio-go-runner is to allow the use of private infrastructure to run TensorFlow workloads.

The primary goal of studio-go-runner is to reduce costs for TensorFlow projects via private infrstructure.

This tool is intended to be used as a statically compiled version of the python runner using Go from Google.  It is intended to be used to run TensorFlow workloads using datacenter infrastructure with the experimenter controlling any dependencies on public or cloud based infrastructure.

Using the studio-go-runner (runner) with the open source studioml tooling can be done without making changes to studioml.  Any configuration needed to use self hosted storage can be made using the studioml yaml configuration file.

The runner is designed to run within multiple styles of deployment configurations.  A reference deployment is used by Sentient that is used within the documentation provided by this repository.

studioml orchestrates the execution of TensorFlow jobs using two types of resources.  Firstly a message queue a used to submit TensorFlow tasks that studioml compliant runners can retrieve and process.  Secondly studioml stores artifacts, namely files, within a storage service.

studioml supports hosted queues offered by cloud providers, namely AWS and Google cloud.  The storage features of studioml are compatible with both cloud providers, and privately hosted storage services using the AWS S3 V4 API.  The studioml python based runner is capable of running on private infrastructure but requires cloud based storage services and cloud based compute instances.

This present runner is capable of supporting several additional features beyond that of the studioml runner:

1. Makes use of privately hosted S3 compatible storage services such as minio.io
2. (future) Makes use of static compute instances that provision GPUs that are shared across multiple studioml experiments
3. (future) Allow runners to interact with studioml API servers to retrieve meta-data related to TensorFlow studioml projects

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

This code based makes use of Go 1.9 which can be downloaded from golang.org.
go dep is used as the dependency management tool.  You do not need to use this tool except during active development. go dep software, and its installation instructions can be found at https://github.com/golang/dep.  go dep is intended to be absorbed into the go toolchain but for now can be obtained independently if needed.  All dependencies for this code base are checked into github following the best practice suggested at https://www.youtube.com/watch?v=eZwR8qr2BfI.

# Runtime Environment
studioml uses the python virtual environment tools to deploy python applications and uses no isolation other than that offered by python.

The go based runner can make use of Singularity, a container platform, to provide isolation and also access to low level machine resources such as GPU cards.  This fuctionality is what differentiates the go based runner from the python based runners that are found within the open source studioml offering.  Singlularity support is offered as an extension to the studioml ecosystem however using its use while visible to studioml affects it in no way.

# Data storage support

The runner supports both S3 V4 and Google Cloud storage models.

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
