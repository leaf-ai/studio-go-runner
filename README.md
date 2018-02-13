# studio-go-runner

studio-go-runner is an implementation of a runner for the Sentient deployments of studioml.

The primary role of studio-go-runner is to allow the use of private infrastructure to run TensorFlow GPU workloads.

The primary goal of studio-go-runner is to reduce costs for TensorFlow projects via private infrstructure.

Version: <repo-version>0.0.33-84-85-fleet-deployments-1eleRs</repo-version>

This tool is intended to be used as a statically compiled version of the python runner using Go from Google.  It is intended to be used to run TensorFlow workloads using datacenter infrastructure with the experimenter controlling storage dependencies on public or cloud based infrastructure.  The studio-go-runner still uses the Google pubSub and Firebase service to allow studio clients to marshall requests.

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

```
    git clone https://github.com/studioml/studio.git
    cd studio
    export GOPATH=`pwd`
    export PATH=~/studio/bin:$PATH
    mkdir -p src/github.com/SentientTechnologies
    cd src/github.com/SentientTechnologies
    git clone https://github.com/SentientTechnologies/studio-go-runner.git
    cd studio-go-runner
```

Code can be executed in one of two ways via docker based builds (please see the compilation section), or using the 'go build' command.

```
    go run cmd/runner/main.go
```

# Compilation

This code based makes use of Go 1.9.  The compiler can be found on the golang.org web site for downloading.

go dep is used as the dependency management tool.  You do not need to use this tool except during active development. go dep software, and its installation instructions can be found at https://github.com/golang/dep.  go dep is intended to be absorbed into the go toolchain but for now can be obtained independently if needed.  All dependencies for this code base are checked into github following the best practice suggested at https://www.youtube.com/watch?v=eZwR8qr2BfI.

In addition to the go dep generated dependencies this software uses the CUDA development 8.0 libraries.

To deploy version managed CI/CD for the runner a version management tool is used to process the artifact files and to manage the docker containers within the system.

To install the tools on Ubuntu use the following commands:

```shell
$ wget https://github.com/karlmutch/bump-ver/releases/download/0.0.0/bump-ver
$ chmod +x bump-ver
```

Releasing the service using versioning for Docker registries, or cloud provider registries requires first that the version for release is tagged with the desired version using the bump-ver tool to first branch the README.md and other files and then to tag docker repositories.

```shell
$ bump-ver -f README.md dev|patch|minor|major
$ version=`bump-ver extract`
```

In order to asist with builds and deploying the runner a Dockerfile is provided to allow for builds without extensive setup.  The Dockerfile requires Docker CE 17.06 to build the runner.  The first command only needs to be run when the compilation tools, or CUDA version is updated, it is lengthy and typically takes 30 minutes but is only needed once.  The second command can be rerun everytime the source code changes quickly to perform builds.

```
docker build -t runner:$version --build-arg USER=$USER --build-arg USER_ID=`id -u $USER` --build-arg USER_GROUP_ID=`id -g $USER` -f <(bump-ver -t ./Dockerfile -f ./README.md inject))
go get -u github.com/golang/dep/cmd/dep
dep ensure
docker run -v $GOPATH:/project runner:$version
```

If you are performing a release for a build then the GITHUB_TOKEN environment must be set in order for the github release to be pushed correctly.  In these cases the command line would appear as follows:

```
docker run -e GITHUB_TOKEN=$GITHUB_TOKEN -e -v $GOPATH:/project runner:$version
```

After the container from the run completes you will find a runner binary file in the $GOPATH/src/github.com/SentientTechnologies/studio-go-runner/bin directory.

# Running go runner

## Prerequisites

When using ubuntu the following GCC compilers and tools need to be installed to support the C++ and C code embeeded within the python machine learning frameworks being used:

```
sudo apt-get update
sudo apt-get install gcc-4.8 g++-4.8
sudo apt-get install libhdf5-dev liblapack-dev libstdc++6 libc6
```

studioml uses the python virtual environment tools to deploy python applications and uses no isolation other than that offered by python.

nvidia installation should be done on the runner, the following URLs point at the software that needs installation.

You will need to download the cudnn 7.0 and 6.0 librarys from the Nvidia developers website.

https://developer.nvidia.com/compute/machine-learning/cudnn/secure/v6/prod/8.0_20170307/Ubuntu16_04_x64/libcudnn6_6.0.20-1+cuda8.0_amd64-deb
https://developer.nvidia.com/compute/machine-learning/cudnn/secure/v7/prod/8.0_20170802/Ubuntu14_04_x64/libcudnn7_7.0.1.13-1+cuda8.0_amd64-deb

```
wget https://developer.nvidia.com/compute/cuda/8.0/Prod2/local_installers/cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb
mv cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb¶ cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64.deb¶
dpkg -i cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64.deb¶
apt-get update
apt-get install -y cuda
mv libcudnn6_6.0.20-1+cuda8.0_amd64-deb libcudnn6_6.0.20-1+cuda8.0_amd64.deb
dpkg -i libcudnn6_6.0.20-1+cuda8.0_amd64.deb
mv libcudnn7_7.0.1.13-1+cuda8.0_amd64-deb libcudnn7_7.0.1.13-1+cuda8.0_amd64.deb
dpkg -i libcudnn7_7.0.1.13-1+cuda8.0_amd64.deb
```
python 2.7 must be installed as a prerequiste and a pip install should be done for the following wheel file:

```
sudo -H pip install -q pipenv Cython grpcio google-api-python-client google-cloud-storage google-cloud-pubsub google-cloud-core
sudo -H pip install -q https://storage.googleapis.com/tensorflow/linux/gpu/tensorflow_gpu-1.3-cp27-none-linux_x86_64.whl
sudo -H pip install -q scipy numpy scikit-learn h5py keras
sudo -H pip install -q http://download.pytorch.org/whl/cu80/torch-0.2.0.post3-cp27-cp27mu-manylinux1_x86_64.whl 
sudo -H pip install -q torchvision
sudo -H pip install -q pyopenssl --upgrade
```

The go based runner can make use of Singularity, a container platform, to provide isolation and also access to low level machine resources such as GPU cards.  This fuctionality is what differentiates the go based runner from the python based runners that are found within the open source studioml offering.  Singlularity support is offered as an extension to the studioml ecosystem however using its use while visible to studioml affects it in no way.

## Cloud support

The Go and the Python runner found within the reference implementation of StudioML have been tested on the Microsoft Azure cloud.

Azure can run Kubernetes as a platform for fleet management of machines and ace-engine is the preferred means of doing this, at least until AKS can support machine types that have GPU resources.

Instructions on getting started with the azure tooling needed for operating your resources can be found as follows:

- AZ CLI https://github.com/Azure/azure-cli#installation
- acs-engine https://github.com/Azure/acs-engine/blob/master/docs/acsengine.md#install-acs-engine

If you are a developer wishing to push workloads to the Azure Container Service you can find more information at, https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-docker-cli.

If Azure is being used then an Azure account will need and you need to authenticate with the account using the 'az login' command.  This will also require access to a browser to complete the login:

```shell
$ az login
To sign in, use a web browser to open the page https://aka.ms/devicelogin and enter the code B8UVHGH9D to authenticate.
```

Once the main login has been completed you will be able to login to the container registry and other Azure services.  Container registries are named in the global namespace for Azure, if you need to create a registry then this is best done through the portal rather than the CLI as you will need to mess with RBAC permissions once done to shared docker images.

```shell
$ az acr login --name sentientai
Login Succeeded
```

Resource groups are an organizing abstraction within Azure so when using the az command line tools you will need to be aware of the resource group you are operating within.

```
$ az acr list --resource-group studioml --query "[].{acrLoginServer:loginServer}" --output table
AcrLoginServer
---------------------
sentientai.azurecr.io
```

Pushing to Azure then becomes a process of tagging the image locally prior to the push to reflect the Azure login server, as follows

```shell
$ docker tag sentient.ai/studio-go-runner:0.0.33 sentientai.azurecr.io/sentient.ai/studio-go-runner:0.0.33
$ docker push sentientai.azurecr.io/sentient.ai/studio-go-runner:0.0.33-master-1elHeQ
The push refers to a repository [sentientai.azurecr.io/sentient.ai/studio-go-runner]
3080c9e99778: Pushed
dff0a506ff15: Pushed
08f61b0c0de5: Pushed
3e4d13d66a55: Pushed
f9e1cf98a7fc: Pushed
1363a12f250c: Pushed
6f4ce6b88849: Pushed
92914665e7f6: Pushed
c98ef191df4b: Pushed
9c7183e0ea88: Pushed
ff986b10a018: Pushed
0.0.33: digest: sha256:4090e69a59c811f40bf9eb2032a96d185c8007ededa7af82e0e7900e41c97e9a size: 2616
```

Azure image repositories can be queried using the CLI tool, for example:

```shell
$ az acr repository show-tags --name sentientai --repository sentient.ai/studio-go-runner --output table
Result
--------------------
0.0.33-master-1elHeQ
```

The acs-engine tool is now used to create a Kubernetes cluster.  Within Azure, acs-engine acts much like kops does for AWS.  Like kops acs-engine will read a template, see examples/azure/kubernetes.json, and will fill in the account related information and write the resulting Azure Resource Manager templates into the '_output' directory.  The output directory will end up containing things such as SSH keys, k8s configuration files etc.  The kubeconfig files will be generated for each region the service can be deployed to, when using the kubectl tools set your KUBECONFIG environment variable to point at the desired region.  This will happen even if the region is specified using the --location command.

For information related to GPU workloads and k8s please review the following github page, https://github.com/Azure/acs-engine/blob/master/docs/kubernetes/gpu.md.  Using his methodology means not having to be concerned abouyt sping up the nivida plugins and the like.

## Options

The runner supports command options being specified on the command line as well as by using environment variables.  Any command line option can be used within the environment variables by using all capitals and underscores in place of dashes.

### Credentials management

The runner uses a credentials options, --certs-dir, to point at a directory location into which credentials for accessing cloud based queue and storage resources can be placed.  In order to manage the queues that runners will pull work from an orchestration system such as salt should be used to manage the credentials files appearing in this directory.  Adding and removing credentials enables administration of which queues the runners on individual machines will be interacting with.

The existance of a credentials file will trigger the runner to list the queue subscriptions that are accessible to each credential and to then immediately begin pulling work from the same.

### Google PubSub and authentication

The runner makes use of the google PubSub messaging platform to pass work requests from the studioml client to the runner.

The PubSub mode uses an environment variable GOOGLE_APPLICATION_CREDENTIALS, which points at the json credential file, to configure both the google cloud project and to setup the access needed.  The runner will query the project for a list of subscriptions and will then query the subscriptions for work.

At the moment go runner needs a cache directory to function correctly:
```
mkdir /tmp/go-runner-cache
```
An example of a runner command line would look like the following:
```
GOOGLE_APPLICATION_CREDENTIALS=/home/kmutch/.ssh/google-app-auth.json ./runner --cache-dir=/tmp/go-runner-cache --cache-size=1000000000
```

### Logging

The runner does support options for logging and monitoring.  For logging the logxi package options are available.  For example to print logging for debugging purposes the following variables could also be set in addition to the above example:

```
LOGXI_FORMAT=happy,maxcol=1024 LOGXI=*
```

### Slack reporting

The reporting of job results in slack can be done using the go runner.  The slack-hook option can be used to specify a hook URL, and the slack-room option can be used to specify the destination of tracking messages from the runner.

### Device Selection

The go runner supports CUDA_VISIBLE_DEVICES as a means by which the runner can be restricted to the use of specific GPUs within a machine.

Options CPU_ONLY, MAX_CORES, MAX_MEM, MAX_DISK and also be used to restrict the types and magnitude of jobs accepted.

# Data storage support

The runner supports both S3 V4 and Google Cloud storage platforms.  The studioml client is responsible for passing credentials down to the runner using the studioml configuration file.

Google storage allows for public, or private google cloud data to be used with the go runner with a single set of credentials.

A studioml client yaml configuration file for google firebase storage can be specified like the following:

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
    email: xxxx@example.com
    password p8ssw0rd

storage:
    type: gcloud
    bucket: "tfstudio-a8367.appspot.com"

saveWorkspaceFrequency: 1m
experimentLifetime: 48h # The time after which the experiment is deemed to be abandoned and should be killed
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
    email: xxxx@example.com
    password p8ssw0rd

storage:
    type: s3
    endpoint: s3-us-west-2.amazonaws.com
    bucket: "karl-mutch"

saveWorkspaceFrequency: 1m
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
