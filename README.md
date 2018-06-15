# studio-go-runner

studio-go-runner is an implementation of a runner for deployments of studioml, in addition to any other Python dervied workloads.

The primary role of studio-go-runner is to allow the use of private infrastructure to run Deep Learning and Nuero evolution GPU workloads.

The primary goal of studio-go-runner is to reduce costs for TensorFlow projects via private infrastructure.

StudioML allows the creation of python work loads that can be queued using a variety of queuing technologies and input data along with results to be persisted using common storage platforms.

Version: <repo-version>0.4.1-bugfix-107-nvidia-check-1fTb47</repo-version>

This tool is intended to be used as a statically compiled version of the python runner implemented using Go.  It is intended to be used to run TensorFlow workloads using private cloud or datacenter infrastructure with the experimenter controlling storage dependencies on public or cloud based infrastructure.  The studio-go-runner still uses the Google pubSub and Firebase service to allow studio clients to marshall requests.

Using the studio-go-runner (runner) with the open source studioml tooling can be done without making changes to studioml.  Any configuration needed to use self hosted storage can be made using the studioml yaml configuration file.

The runner is designed to run within multiple styles of deployment configurations.  A reference deployment is used by Sentient that is used within the documentation provided by this repository.

studioml orchestrates the execution of TensorFlow jobs using two types of resources.  Firstly a message queue a used to submit TensorFlow tasks that studioml compliant runners can retrieve and process.  Secondly studioml stores artifacts, namely files, within a storage service.

studioml supports hosted queues offered by cloud providers, namely AWS and Google cloud.   StudioML also supports privately provisioned queues using RabbitMQ.  The storage features of studioml are compatible with both cloud providers, and privately hosted storage services using the AWS S3 V4 API.

This present runner is capable of supporting several additional features beyond that of the studioml runner:

1. Makes use of privately hosted S3 compatible storage services such as minio.io
2. (future) Makes use of static compute instances that provision GPUs that are shared across multiple studioml experiments
3. (future) Allow runners to interact with studioml API servers to retrieve meta-data related to TensorFlow studioml projects

# Using releases

The studio go runner (runner) primary release vehicle is Github.  You will find a statically line amd64 binary executable on Github.  This exectable can be packed into docker containers for those wishing to do their own solutions integration.  The runner is also available to solutions partners using Docker images that are specific to the solution Sentiant and its partners are using to deliver turn key deployments.

# Using the code

The github repository should be cloned an existing git clone of the https://github.com/studioml/studio.git repo.  Within the studio directories create a sub directory src and set your GOPATH to point at the top level studio directory.

```
mkdir ~/project
cd ~/project
export GOPATH=`pwd`
export PATH=$GOPATH/bin:$PATH
mkdir -p src/github.com/SentientTechnologies
cd src/github.com/SentientTechnologies
git clone https://github.com/SentientTechnologies/studio-go-runner.git
cd studio-go-runner
```

Code can be executed in one of two ways via docker based builds (please see the compilation section), or using the 'go build' command.

```
go run cmd/runner/main.go
```

## Compilation

This code based makes use of Go 1.10.  The compiler can be found on the golang.org web site for downloading. On ubuntu the following command can be used:
```
sudo apt-get install golang-1.10
```

go dep is used as the dependency management tool.  You do not need to use this tool except during active development. go dep software, and its installation instructions can be found at https://github.com/golang/dep.  go dep is intended to be absorbed into the go toolchain but for now can be obtained independently if needed.  All dependencies for this code base are checked into github following the best practice suggested at https://www.youtube.com/watch?v=eZwR8qr2BfI.

In addition to the go dep generated dependencies this software uses the CUDA development 8.0 libraries.

To deploy version managed CI/CD for the runner a version management tool is used to process the artifact files and to manage the docker containers within the system.

To install the tools on Ubuntu use the following commands:

```shell
mkdir -p $GOPATH/bin
wget -O $GOPATH/bin/semver https://github.com/karlmutch/duat/releases/download/0.7.0/semver-linux-amd64
wget -O $GOPATH/bin/stencil https://github.com/karlmutch/duat/releases/download/0.7.0/stencil-linux-amd64
wget -O $GOPATH/bin/github-release https://github.com/karlmutch/duat/releases/download/0.7.0/github-release-linux-amd64
chmod +x $GOPATH/bin/semver
chmod +x $GOPATH/bin/stencil
chmod +x $GOPATH/bin/github-release
go get -u github.com/golang/dep/cmd/dep
```

Releasing the service using versioning for Docker registries, or cloud provider registries requires first that the version for release is tagged with the desired version using the semver tool to first brand the README.md and other files and then to tag docker repositories.

In order to asist with builds and deploying the runner a Dockerfile is provided to allow for builds without extensive setup.  The Dockerfile requires Docker CE 17.06, or later, to build the runner.  The first command only needs to be run when the compilation tools, or CUDA version is updated, it is lengthy and typically takes 30 minutes but is only needed once.  The docker run command can be rerun everytime the source code changes quickly to perform builds.

The build tool will produce a list of binaries produced by the build that can be feed into tools such as github-release from duat.  The form of the docker run in the following example is what should be used when the release processing is not being done within the container.  The piped commands will correct the output file names printed for the environment outside of the container context.

```
dep ensure
stencil < Dockerfile | docker build -t runner-build --build-arg USER=$USER --build-arg USER_ID=`id -u $USER` --build-arg USER_GROUP_ID=`id -g $USER` -
docker run -v $GOPATH:/project runner-build | sed 's/\/project\//$GOPATH\//g'| envsubst
```

If you are performing a release for a build using the containerize build then the GITHUB_TOKEN environment must be set in order for the github release to be pushed correctly.  In these cases the command line would appear as follows:

```
docker run -e GITHUB_TOKEN=$GITHUB_TOKEN -v $GOPATH:/project runner-build | sed 's/\/project\//$GOPATH\//g'| envsubst
```

After the container from the run completes you will find a runner binary file in the $GOPATH/src/github.com/SentientTechnologies/studio-go-runner/bin directory.

In order to create containerized version of the runner you will need to make use of the build.go tool and this requires that go 1.10 or later to be installed.  Ubuntu instructions can be found for go 1.10 at, https://github.com/golang/go/wiki/Ubuntu.To produce a tagged container for the studio go runner use the following command, outside of container which will allow the containerization step to run automatically:

```
go run ./build.go -r
```

# Running go runner

The go runner has been designed to be adaptive to run in any type of deployment environment, cloud, on-premise for in VM infrastructure.  The following sections describe some reference deploymebnt styles that are being used on a regular basis.  If you wish for a different deployment model please talk with a Sentient staff member for guidence.

## Non containerized deployments

When using ubuntu the following GCC compilers and tools need to be installed to support the C++ and C code embeeded within the python machine learning frameworks being used:

```
sudo apt-get update
sudo apt-get install gcc-4.8 g++-4.8
sudo apt-get install libhdf5-dev liblapack-dev libstdc++6 libc6
```

studioml uses the python virtual environment tools to deploy python applications and uses no isolation other than that offered by python.

nvidia installation should be done on the runner, the following URLs point at the software that needs installation.

You will need to download the cudnn 7.0 and 6.0 librarys from the Nvidia developers website.

https://developer.nvidia.com/compute/machine-learning/cudnn/secure/v6/prod/8.0\_20170307/Ubuntu16\_04\_x64/libcudnn6\_6.0.20-1+cuda8.0\_amd64-deb
https://developer.nvidia.com/compute/machine-learning/cudnn/secure/v7/prod/8.0\_20170802/Ubuntu14\_04\_x64/libcudnn7\_7.0.1.13-1+cuda8.0\_amd64-deb

```
wget https://developer.nvidia.com/compute/cuda/8.0/Prod2/local_installers/cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb
mv cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64.deb
dpkg -i cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64.deb
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
sudo -H pip install -q https://storage.googleapis.com/tensorflow/linux/gpu/tensorflow_gpu-1.4.1-cp27-none-linux_x86_64.whl
sudo -H pip install -q scipy numpy scikit-learn h5py keras
sudo -H pip install -q http://download.pytorch.org/whl/cu80/torch-0.2.0.post3-cp27-cp27mu-manylinux1_x86_64.whl 
sudo -H pip install -q torchvision
sudo -H pip install -q pyopenssl --upgrade
```

The go based runner can make use of Singularity, a container platform, to provide isolation and also access to low level machine resources such as GPU cards.  This fuctionality is what differentiates the go based runner from the python based runners that are found within the open source studioml offering.  Singlularity support is offered as an extension to the studioml ecosystem however using its use while visible to studioml affects it in no way.

Having completed the initial setup steps you should visit the https://github.com/SentientTechnologies/studio-go-runner/releases page and download the appropriate version of the runner and use it directly.

## Containerized deployments

The runner can be deployed using a container registry within cloud or on-premise environments.  The runner code comes bundled with a Dockerfile within the cmd/runner directory that can be used to generate your own images for deployment into custom solutions.

If you are using StudioML in conjunction with Sentient Technologies it is highly likely that a Container Registry has already been made available.  Talk with Sentient staff about the exact arrangements and your requirements for software deployment.

Containerized workloads are orchestrated using Kubernetes.  The cloud based deployments for the go runner employ Kubernetes in order to maintain vendor neutrality and reduce support complexity.  If you wish to make use of vendor specific container orchestration frameworks, for example AWS FarGate, you will need to use the vendor specific tooling which while possible, does not fall within the scope of this document.

## Kubernetes (k8s) tools installation

Installations of k8s can use both the kops (AWS), and the kubectl tools. When creating a cluster of machines these tools will be needed to provision the core cluster with the container orchestration software.

These tools will be used from your workstation and will operate on the k8s cluster from a distance.

### Verify Docker Version

Docker is preinstalled.  You can verify the version by running the following:
<pre><code><b>docker --version</b>
Docker version 17.12.0-ce, build c97c6d6
</code></pre>
You should have a similar or newer version.
## Install Kubectl CLI

Install the kubectl CLI can be done using any 1.9.x version.

<pre><code><b> curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/v1.9.7/bin/linux/amd64/kubectl && chmod +x kubectl && sudo mv kubectl /usr/local/bin/</b>
</code></pre>

Add kubectl autocompletion to your current shell:

<pre><code><b>source <(kubectl completion bash)</b>
</code></pre>

You can verify that kubectl is installed by executing the following command:

<pre><code><b>kubectl version --client</b>
Client Version: version.Info{Major:"1", Minor:"9", GitVersion:"v1.9.2", GitCommit:"5fa2db2bd46ac79e5e00a4e6ed24191080aa463b", GitTreeState:"clean", BuildDate:"2018-01-18T10:09:24Z", GoVersion:"go1.9.2", Compiler:"gc", Platform:"linux/amd64"}
</code></pre>

## Creating Kubernetes clusters

Kubernetes clusters can be created using a variety of tools.  Within AWS the preferred tool is the Kubenertes open source kops tool.  To read how to make use of this tool please refer to the docs/aws.md file for additional information.  The Azure specific instructions are detailed in docs/azure.md.

After your cluster has been created you can use the instructions within the next sections to interact with your cluster.

## Kubernetes setup

### Kubernetes Web UI and console

In addition to the kops information for a cluster is hosted on S3, the kubectl information for accessing the cluster is stored within the ~/.kube directory.  The web UI can be deployed using the instruction at https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard/#deploying-the-dashboard-ui, the following set of instructions include the deployment as it stood at k8s 1.9.  Take the opportunity to also review the document at the above location.

Kubectl service accounts can be created at will and given access to cluster resources.  To create, authorize and then authenticate a service account the following steps can be used:

```
kubectl create -f https://raw.githubusercontent.com/kubernetes/heapster/release-1.5/deploy/kube-config/influxdb/influxdb.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes/heapster/release-1.5/deploy/kube-config/influxdb/heapster.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes/heapster/release-1.5/deploy/kube-config/influxdb/grafana.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes/heapster/release-1.5/deploy/kube-config/rbac/heapster-rbac.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes/dashboard/master/src/deploy/recommended/kubernetes-dashboard.yaml
kubectl create serviceaccount studioadmin
secret_name=`kubectl get serviceaccounts studioadmin -o json | jq '.secrets[] | [.name] | join("")' -r`
secret_kube=`kubectl get secret $secret_name -o json | jq '.data.token' -r | base64 --decode`
# The following will open up all service accounts for admin, review the k8s documentation specific to your
# install version of k8s to narrow the roles
kubectl create clusterrolebinding serviceaccounts-cluster-admin --clusterrole=cluster-admin --group=system:serviceaccounts
```

The value in secret kube can be used to login to the k8s web UI.  First start 'kube proxy' in a terminal window to create a proxy server for the cluster.  Use a browser to navigate to http://localhost:8001/ui.  Then use the value in the secret\_kube variable as your 'Token' (Service Account Bearer Token).

You will now have access to the Web UI for your cluster with full privs.

## Runner Kubernetes setup

Having created a cluster the following instructions will guide you through deploying the runner into the cluster in a cloud neutral way.

## runner configuration

The runner can be configured using environment variables.  To do this you will find kubernetes configuration maps inside the example deployment files provided within this git repository.  Any command line variables used by the runner can also be supplied as environment variables by changing any dashes '-' to underscores '\_', and by using upper case names.

Be sure to review any yaml deployment files you are using or are given prior to using 'kubectl apply' to push this configuration data into your studioml clusters.  For more information about the use of kubernetes configuration maps please review the foloowing useful article, https://akomljen.com/kubernetes-environment-variables/.

In order to selectively run queues a combination of the queue prefix specified in a configMap within your kubernetes deployment yaml file, and when starting your studioml client can be used to isolate your own work.

### Kubernetes Secrets and the runner

The runner is able to accept credentials for accessing queues via the running containers file system.  To interact with a runner cluster deployed on kubernetes the kubectl apply command can be used to inject the credentials files into the filesystem of running containers.  This is done by extracting the json (google cloud credentials), that encapsulate the credentials and then running the base64 command on it, then feeding the result into a yaml snippet that is then applied to the cluster instance using kubectl appl -f as follows:

```shell
$ google_secret=`cat certs/google-app-auth.json | base64 -w 0`
$ kubectl apply -f <(cat <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: studioml-runner-google-cert
type: Opaque
data:
  google-app-auth.json: $google_secret
EOF
)
secret "studioml-runner-google-cert" created
$ aws_sqs_cred=`cat ~/.aws/credentials | base64 -w 0`
$ aws_sqs_config=`cat ~/.aws/config | base64 -w 0`
$ kubectl apply -f <(cat <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: studioml-runner-aws-sqs
type: Opaque
data:
  credentials: $aws_sqs_cred
  config: $aws_sqs_config
EOF
)
```

Be aware that any person, or entity having access to the kubernetes vault can extract these secrets unless extra measures are taken to first encrypt the secrets before injecting them into the cluster.
For more information as to how to used secrets hosted through the file system on a running k8s container please refer to, https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets-as-files-from-a-pod.

## Runner deployment

```shell
$ kubectl apply -f <(stencil < examples/azure/deployment-1.9.yaml)
deployment "studioml-go-runner" created
$ kubectl get pods
NAME                                  READY     STATUS              RESTARTS   AGE
studioml-go-runner-1428762262-456zg   0/1       ContainerCreating   0          24s
$ kubectl describe pods
... returns really useful container orchestration information should anything go wrong ...
$ kubectl get pods
NAME                                  READY     STATUS              RESTARTS   AGE

```


## Options

The runner supports command options being specified on the command line as well as by using environment variables.  Any command line option can be used within the environment variables by using all capitals and underscores in place of dashes.

### Credentials management

The runner uses a credentials options, --certs-dir, to point at a directory location into which credentials for accessing cloud based queue and storage resources can be placed.  In order to manage the queues that runners will pull work from an orchestration system such as kubernetes, or salt should be used to manage the credentials files appearing in this directory.  Adding and removing credentials enables administration of which queues the runners on individual machines will be interacting with.

The existance of a credentials file will trigger the runner to list the queue subscriptions that are accessible to each credential and to then immediately begin pulling work from the same.

### Google PubSub and authentication

The runner can make use of google PubSub messaging platform to pass work requests from the studioml client to the runner.  The runner while compatible with the Google Cloud Platform has not specific deployment instructions at this point.  These instructions relate to accessing Googles PubSub queue facility from outside of the Google cloud.

The PubSub mode uses an environment variable GOOGLE\_APPLICATION\_CREDENTIALS, which points at the json credential file, to configure both the google cloud project and to setup the access needed.  The runner will query the project for a list of subscriptions and will then query the subscriptions for work.

At the moment go runner needs a cache directory to function correctly:
```
mkdir /tmp/go-runner-cache
```
An example of a runner command line would look like the following:
```
GOOGLE_APPLICATION_CREDENTIALS=/home/kmutch/.ssh/google-app-auth.json ./runner --cache-dir=/tmp/go-runner-cache --cache-size=1000000000
```

### AWS SQS and authentication

AWS queues can also be used to queue work for runners, regardless of the cloud that was used to deploy the runner.  The credentials in a data center or cloud environment will be stored using files within the container or orchestration run time.

The AWS credentials are deployed using files for each credential within the directory specified by the --sqs-certs option.

### RabbitMQ access

RabbitMQ is supported by StudioML and the golang runner and an alternative to SQS, and Goodle PubSub.  To make use of rabbitMQ a url should be included in the studioML configuration file that details the message queue.  For example:

```
cloud:
    queue:
        uri: "amqp://guest:guest@localhost:5672/"
```

RabbitMQ within the various cloud providers is well supported either using direct installation on compute instances or by using packagers such as Bitnami.  You will find instructions for Azure in this regard in the docs/azure.md file.

When the job is run from Studio specify a queue name that begins with rmq\_ and the system will located the cloud -> queue -> uri reference and treat it as a RabbitMQ queue.  The runner likewise will also accept the uri using the following option:

```
    -amqp-url string
        The URI for an amqp message exchange through which StudioML is being sent (default "amqp://guest:guest@localhost:5672/")
```

RabbitMQ is used in situations where cloud based queuing is either not available or not wanted.

Before using rabbitMQ a password should be set and the guest account disabled to protect the queuing resources and the information being sent across these queues.

### Logging

The runner does support options for logging and monitoring.  For logging the logxi package options are available.  For example to print logging for debugging purposes the following variables could also be set in addition to the above example:

```
LOGXI8FORMAT=happy,maxcol=1024 LOGXI=*
```

### Slack reporting

The reporting of job results in slack can be done using the go runner.  The slack-hook option can be used to specify a hook URL, and the slack-room option can be used to specify the destination of tracking messages from the runner.

### Device Selection

The go runner supports CUDA\_VISIBLE\_DEVICES as a means by which the runner can be restricted to the use of specific GPUs within a machine.

Options CPU\_ONLY, MAX\_CORES, MAX\_MEM, MAX\_DISK and also be used to restrict the types and magnitude of jobs accepted.

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
