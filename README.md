<p style="font-size: 2em;margin: .67em 0">studio-go-runner</p>

Version: <repo-version>0.9.27-master-aaaagnshkci</repo-version>

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/leaf-ai/studio-go-runner/blob/master/LICENSE) [![Go Report Card](https://goreportcard.com/badge/leaf-ai/studio-go-runner)](https://goreportcard.com/report/leaf-ai/studio-go-runner)

studio-go-runner, or runner, is an implementation of a StudioML runner enhanced for use with neuro-evolutionary (ENN) experiments.  runner continues to support any Python derived workloads in the same manner as the StudioML python runner.

The primary purpose of the runner is to provide enabling infrastructure to improve the efficency, and lightweight user experience of running large scale, trusted neuro-evolution experiments.

The primary role of the runner is to provide an execution platform for AI experiments generally and ENN experiments specifically.

The primary function of the runner is to run workloads within public and private infrastructure reducing the cost of managing larger scale AI projects.

Actors in the runner ecosystem :

A queuing component for orchestrating experiments on remote workers.  StudioML allows the creation of python work loads that can be queued using a variety of message queue technologies and input data along with results to be persisted and shared using common storage platforms.

A storage complex for hosting experiment source data and experiment results, typically an S3 compatible storage offering.  ENN reporter is an envisioned element of the ecosystem that will perform a similar function to the 'studio ui' command with additional features to cache queries against the S3 or other system of record for experiments and projects.  It will be delivered as a seperate component.

The interactions and design of the StudioML major processing components although well predating the Kubernets [Coarse Parallel Processing Using a Work Queue](https://kubernetes.io/docs/tasks/job/coarse-parallel-processing-work-queue) note align well with it, and also extends to Fine Grained processing as an addition option.

<!--ts-->

Table of Contents
=================

* [Table of Contents](#table-of-contents)
* [Introduction and ENN workflow](#introduction-and-enn-workflow)
* [A note concerning security and privacy](#a-note-concerning-security-and-privacy)
* [Application Notes](#application-notes)
* [Usage](#usage)
* [Platform Documentation](#platform-documentation)
* [Kubernetes tooling install](#kubernetes-tooling-install)
  * [Kubernetes installations](#kubernetes-installations)
  * [Verify Docker Version](#verify-docker-version)
  * [Install Kubectl CLI](#install-kubectl-cli)
* [Docker Desktop deployments](#docker-desktop-deployments)
* [Kubernetes (k8s) based deployments](#kubernetes-k8s-based-deployments)
  * [Creating Kubernetes clusters](#creating-kubernetes-clusters)
  * [Kubernetes setup](#kubernetes-setup)
  * [Kubernetes Web UI and console](#kubernetes-web-ui-and-console)
  * [Runner Kubernetes setup](#runner-kubernetes-setup)
    * [runner configuration](#runner-configuration)
  * [Kubernetes Secrets and the runner](#kubernetes-secrets-and-the-runner)
* [Example application](#example-application)
* [Metadata](#metadata)
  * [Introduction](#introduction)
  * [Metadata Details](#metadata-details)
    * [Lifecycle](#lifecycle)
* [Implementation](#implementation)
* [Using releases](#using-releases)
* [Using the code](#using-the-code)
* [Compilation](#compilation)
  * [Prerequisties](#prerequisties)
    * [General Utilities](#general-utilities)
    * [Compilation Tools](#compilation-tools)
* [Running go runner  (Standalone)](#running-go-runner--standalone)
  * [Non containerized deployments](#non-containerized-deployments)
  * [Containerized deployments](#containerized-deployments)
* [Options and configuration](#options-and-configuration)
  * [Cloud support](#cloud-support)
  * [Credentials management](#credentials-management)
  * [AWS SQS and authentication](#aws-sqs-and-authentication)
  * [RabbitMQ access](#rabbitmq-access)
  * [Logging](#logging)
  * [Slack reporting](#slack-reporting)
  * [Device Selection](#device-selection)
* [Data storage support](#data-storage-support)
<!--te-->
# Introduction and ENN workflow

The runner commencing with version 1.x marks a progression from simple StudioML compatibility to supporting ENN use cases specifically, enabling a pipeline based approach in conjunction with ENN AI that smooths the application of ENN to model creation to address business problems.

Serving models is outside the scope of this specific component of the StudioML ecosystem.  Models created by experimenters using StudioML can of course be deployed across a large number of environments supporting any model formats that experiments choose to support.

The runner in addition to supporting StudioML workflows introduces several features useful for projects that produce large numbers of automatically managed models without burdening the experimenter.

In the case of ENN the common workflow involves starting and maintaining a multitude of experiments each consisting of multiple phases, with a fan-out fan-in work unit pattern.  To maintain this type of project the experimenter starts a long running python task that uses the StudioML completion service python class to initiate a number of experiments and then waits on their completion.  On completion of sufficient individual experiments the experiment code evaluates the results and makes decisions for initiating the next wave, or generation, of experiments.  The python StudioML runner offered in the main StudioML python offering however, is optimized toward the use case of a single phase of experiments followed by the manual curation of the results by a human experimenter, hyper-parameter searches are one example of this. Standalone StudioML use cases deliver logs and tensorboard outputs as artifacts that are curated by experimenters, typically on S3 style infrastructure.

To run multiple generations within a project StudioML provides a python class, CompletionService, as an example of a strategy for handling experiments in a fan-out, fan-in workflow.  The runner augments experiment runs with artifacts that can assist project developers and experimenters with support for Linux containers and enhanced reporting artifacts.  The runner also supports tracking of experiment assignments to machines or Kubernetes nodes, as one example of these extensions.  While the python StudioML project supports reporting and model wrangling its use cases are more broadly focused.

Evolutionary neural network (ENN) methodologies create both the topology and weights for neural networks (NN).  The ENN scenario keeps the architectures of networks being evaluated in constant flux.  Networks are constantly created and destroyed.  StudioML can be used to investigate the results of evaluating networks during development of ENN code. However, once the experiment reaches the scale needed to achieve state of the art results individual curation of experiments becomes impractical.  The StudioML go runner addresses these constraints by providing a number of assets that accompany experiments such as metadata and metrics artifacts that can be consumed by downstream experimenter created scripts and tools.

# A note concerning security and privacy

For security and privacy studioml relies on both perimeter and infrastructure security.

For perimeter security the runner should deployed within Kubernetes which offers isolation of components.  The default deployment resource definitions within the example files provided by the repository should be refined by your cluster, network, cloud and security teams to implement appropriate RBAC implementations for the level of compliance needed.  The runner dose not impose any additional requirements other than for highly secure environments each team having different access rights to data used during experiments should be on seperate runner clusters.

For communications paths a combination of having end-to-end encryption using messages queues with auto-encryption can be used (AWS SQS), or messages themselves can be encrypted.  You can also use a combination of both should you wish.

Encryption of the message payloads are described in the [interface definition file](docs/interface.md).  Encryption is only supported within Kubernetes deployments.  The reason for this is that standalone runners cannot be secured and have shared secrets without the isolation features as provided by Kubernetes.

When using Kubernetes that at a minimum a secured image registry is used and that users should use the image signing features of their choosen distribution or cloud offering.  runner images can be obtained from trusted sources, such as Cognizant, or they can be built within your own infrastructure and then signed, then before being moved into a secured private environment after user scanning and analysis is done.  Cloud vendors typically offer these capabilities within their Kubernetes as a service products.

When deploying each use cases will have a variety of custom requirements for permitted operations and privileges needed.  In order to lock down your specific deployment the following materials might help to reveal some of the issues to consider:

* [Seccomp in Kubernetes — Part I: 7 things you should know before you even start!](https://itnext.io/seccomp-in-kubernetes-part-i-7-things-you-should-know-before-you-even-start-97502ad6b6d6)

* [Attack matrix for Kubernetes](https://www.microsoft.com/security/blog/2020/04/02/attack-matrix-kubernetes/)

# Application Notes

Information concerning the use of ML libraries with StudioML can be found in the docs/app-notes directory:

* Theano and numpy and basic linear algebra support for multi-threaded applications [docs/app-notes/numpy-blas.md]

# Usage

The runner is typically packaged within a container image, and installed using a Kubernetes cluster.  Instructions for deployment on specific platforms can be found in the docs directory of this repository.

Installation for retail usage of this software is done typically in the following steps:

1. Select and deploy a Kubernetes cluster distribution
2. Select either SQS or RabbitMQ queuing dependent upon the choice of cloud, or on-premises platform
3. Select either S3 or minio storage also dependent upon the choice of cloud, or on-premises platform
4. Determine your dynamic allocation and cost management solution [optional]
4. Deploy the runner in the form-factor of GPU enabled Kubernetes pods

Once completed experimenters can return to their python experiment hosts to configure their StudioML queue, and storage platforms of choice then launch experiments.

Users of this platform can also leverage the information within the interface.md file to build their own integrations to the runner environment, some of our users for example have written swift clients.

# Platform Documentation

The runner can be deployed to a wide variety of different platforms.  Information concerning the generic Kubernetes deployment is detailed in the next major section.

While specific cloud deployments are detailed using scripting capable CLI commands the cloud vendor specific tools such as AKS for AWS can just as easily be used within the vendors web UI portals.  Prior to using these guides the Kubernetes tooling install steps, below, should have been completed. Information concerning the individual platforms can be found in the following documents:

[AWS Kubernetes support](docs/aws_k8s.md)

[Azure support](docs/azure.md)

Information related to queuing of work for the compute cluster and the storage platform can be found in the following documents:

[Queueing and StudioML](docs/queuing.md)

[Storage and StudioML](docs/storage.md)

[GPU Allocation](docs/gpus.md)

# Kubernetes tooling install

## Kubernetes installations

Installations of k8s can use both the eksctl (AWS), acs-engine (Azure), and the kubectl tools. When creating a cluster of machines these tools will be needed to provision the core cluster with the container orchestration software.

These tools will be used from your workstation and will operate on the k8s cluster created using eksctl, or the azure CLI.

## Verify Docker Version

Docker is preinstalled.  You can verify the version by running the following:
<pre><code><b>docker --version</b>
Docker version 18.09.4, build d14af54
</code></pre>
You should have a similar or newer version.

## Install Kubectl CLI

Detailed instructions for kubectl can be found at, https://kubernetes.io/docs/tasks/tools/install-kubectl/#install-kubectl.

Installing kubectl CLI can be in brief using the following steps:

<pre><code><b> curl -LO https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl
</b></code></pre>

Add kubectl autocompletion to your current shell:

<pre><code><b>source <(kubectl completion $(basename $SHELL)) </b>
</code></pre>

You can verify that kubectl is installed by executing the following command:

<pre><code><b>kubectl version --client</b>
Client Version: version.Info{Major:"1", Minor:"12", GitVersion:"v1.12.2", GitCommit:"17c77c7898218073f14c8d573582e8d2313dc740", GitTreeState:"clean", BuildDate:"2018-10-24T06:54:59Z", GoVersion:"go1.10.4", Compiler:"gc", Platform:"linux/amd64"}
</code></pre>

# Docker Desktop deployments

The go runner is able to be run within a docker desktop environment under Mac and Windows.  Documentation is provided for [docker desktop](https://docs.docker.com/desktop/).

Examples have been created for use of the go runner in the context of a docker desktop environment and can be found at [examples/docker](examples/docker/README.md).

# Kubernetes (k8s) based deployments

The current kubernetes (k8s) support employs Deployment resources to provision pods containing the runner as a worker.  In pod based deployments the pods listen to message queues for work and exist until they are explicitly shutdown using Kubernetes management tools.

Support for using Kubernetes job resources to schedule the runner is planned, along with proposed support for a unified job management framework to support drmaa scheduling for HPC.

## Creating Kubernetes clusters

The runner can be used on vanilla k8s clusters.  The recommended version of k8s is 1.14.9, at a minimum version for GPU compute.  k8s 1.14 can be used reliably for CPU workloads.

Kubernetes clusters can be created using a variety of tools.  Within AWS the preferred tool is the Kubenertes open source eksctl tool.  To read how to make use of this tool please refer to the docs/aws.md file for additional information.  The Azure specific instructions are detailed in docs/azure.md.

After your cluster has been created you can use the instructions within the next sections to interact with your cluster.

## Kubernetes setup

It is recommended that prior to using k8s you become familiar with the design concepts.  The following might be of help, https://github.com/kelseyhightower/kubernetes-the-hard-way.

## Kubernetes Web UI and console

In addition to the eksctl information for a cluster is hosted on S3, the kubectl information for accessing the cluster is stored within the ~/.kube directory.  The web UI can be deployed using the instruction at https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard/#deploying-the-dashboard-ui, the following set of instructions include the deployment as it stood at k8s 1.9.  Take the opportunity to also review the document at the above location.

Kubectl service accounts can be created at will and given access to cluster resources.  To create, authorize and then authenticate a service account the following steps can be used:

```
kubectl create -f https://raw.githubusercontent.com/kubernetes/heapster/release-1.5/deploy/kube-config/influxdb/influxdb.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes/heapster/release-1.5/deploy/kube-config/influxdb/heapster.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes/heapster/release-1.5/deploy/kube-config/influxdb/grafana.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes/heapster/release-1.5/deploy/kube-config/rbac/heapster-rbac.yaml
kubectl create -f https://raw.githubusercontent.com/kubernetes/dashboard/v1.10.0/src/deploy/recommended/kubernetes-dashboard.yaml
kubectl create serviceaccount studioadmin
secret_name=`kubectl get serviceaccounts studioadmin -o JSON | jq '.secrets[] | [.name] | join("")' -r`
secret_kube=`kubectl get secret $secret_name -o JSON | jq '.data.token' -r | base64 --decode`
# The following will open up all service accounts for admin, review the k8s documentation specific to your
# install version of k8s to narrow the roles
kubectl create clusterrolebinding serviceaccounts-cluster-admin --clusterrole=cluster-admin --group=system:serviceaccounts
```

The value in secret kube can be used to login to the k8s web UI.  First start 'kube proxy' in a terminal window to create a proxy server for the cluster.  Use a browser to navigate to http://localhost:8001/ui.  Then use the value in the secret\_kube variable as your 'Token' (Service Account Bearer Token).

You will now have access to the Web UI for your cluster with full privs.

## Runner Kubernetes setup

Having created a cluster the following instructions will guide you through deploying the runner into the cluster in a cloud neutral way.

### runner configuration

The runner can be configured using environment variables.  To do this you will find kubernetes configuration maps inside the example deployment files provided within this git repository.  Any command line variables used by the runner can also be supplied as environment variables by changing any dashes '-' to underscores '\_', and by using upper case names.

The follow example shows an example ConfigMap that can be referenced by the k8s Deployment block:

```
apiVersion: v1
kind: ConfigMap
metadata:
  name: studioml-env
data:
  LOGXI_FORMAT: "happy,maxcol=1024"
  LOGXI: "*=DBG"
  SQS_CERTS: "certs/aws-sqs"
  QUEUE_MATCH: "^(rmq|sqs)_.*$"
```

The above options are a good starting point for the runner.  The queue-match option is used to specify a regular expression of what queues will be examined for StudioML work.  If you are running against a message queue server that has mixed workloads you will need to use this option.

Be sure to review any yaml deployment files you are using, or are given prior to using 'kubectl apply' to push this configuration data into your StudioML clusters.  For more information about the use of kubernetes configuration maps please review the foloowing useful article, https://akomljen.com/kubernetes-environment-variables/.

Support for handling the queue processing within runners using a Kubernetes configuration map is documented at, https://github.com/leaf-ai/studio-go-runner/blob/master/docs/k8s.md#configuration-map-support.  If you wish to perform fine grained management of runners maps can be used to select specific runners.

## Kubernetes Secrets and the runner

The runner is able to accept credentials for accessing queues via the running containers file system.  To interact with a runner cluster deployed on kubernetes the kubectl apply command can be used to inject the credentials files into the filesystem of running containers.  This is done by extracting the environment variables etc that encapsulate the credentials and then running the base64 command on them, they are then feed into a yaml snippet that is then applied to the cluster instance using kubectl apply -f.  Detailed instructions for each platform are included in that platforms documentation.

# Example application

A fully worked example application can be found in the examples/aws/cpu directory that consists of steps to run an AWS deployment with hell world python application.

# Metadata

StudioML defines data entities for 'experiment processing' messages as the primary way that StudioML clients dispatch work.  The definition of the message format can be found in the docs/interfaces.md file.

Additional information can be found in this code repository within the docs/metadata.md file.

## Introduction

StudioML metadata associated with a single experiment, if the \_metadata artifact is specified, can be useful to the experimenter with or without a broader project.  Experiments can generate their own data related artifacts directed at metadata artifact files using line oriented JSON patches and objects.  If a mutable \_metadata artifact is defined this causes the runner to scrape these objects and gather them into a single JSON document within the artifact for use by downstream ETL processes and query engines.  This is in addition to other artifacts that already exist including, for example tensorboard data using a tb artifact.

The \_metadata JSON documents include details such as the machine the experiment touched, runner generated JSON tags, and JSON generated at the discretion of the users application.  Multiple runs of the experiment will result in additional files being generated and added to the artifact directory, or key heirarchy in the experimenter configured storage platform.

The JSON payload used to make the experiment request is also written by the runner into the metadata artifact configured by the experimenter.  metadata files within the artifact can then used by experiment management tools to construct a record of the experiments that have occured and their context.  The metadata store, seperate from the metadata artifact, contains a directory of all experiments that have been initiated with a redacted form of the JSON request.  Any tool using the redacted JSON document will need to supply credentials to obtain the contents of the experiment artifacts that were generated.

In order to query against experiment attributes such as evaluated fitness, the metadata experiment manifest is first extracted and read.  Based on URIs inside the experiment manifest any attributes and results from the experiment can be read using its artifacts, given you have credentials to do so.  The project id supplied by the experimenter to StudioML client when initiating the experiment must be unique.  Other information within the experiment including the experiment id is not intended to be unique except within the context of a project.

To support evolutionary learning use cases the plan is to use a combination of the go runner and new additional tooling delivered using leaf-ai open source github projects.  The new runner and ENN features within leaf-ai projects will leave the studio github organization and StudioML projects untouched as much as is possible.

## Metadata Details

While the metadata use case we discuss here is ENN related the addition of general purpose JSON objects as metadata allows for any style of work flow using python or container workloads to be organized and observed.  StudioML and the go runner are designed with the notion in mind that machine learning will change rapidly and so avoid defining a formal relational schema.  The applications using StudioML are instead provided with a means by which metadata can be defined in a well formed manner and extracted by downstream components that implement a specific workflow or business process.  StudioML applications only need to output valid JSON fragments on their standard output and it will be placed into the jobs JSON output artifact ready for ingestion or indexing directly on S3 or other storage platforms.

### Lifecycle

To be able to implement downstream data management however the JSON data being output by experiments needs to conform to two characteristics

1. Idempotent data entities

json tags must only appear once during the duration of an experiment.  By following this principal line oriented JSON objects that are placed into the experiments JSON document can be merged as they appear without the issue of data being updated and transitions being lost or causing an issue due to delayed updates.

2. Eventually consistent

Experiments are considered to be long running entities that refresh data in long cycles. Experiment completion is indicated via the studioml block 'status' tag.

Applications injecting data should use valid JSON line oriented with the top tag of 'experiment' as a JSON object. For example:

```
2018-11-07T01:04:42+0000 INF runner starting run _: [project_id  experiment_id completion_service_rBranch-LEAF-80-a2-test-dir-55822bd0-e107-4f0f-9d1f-bd374fdcff21_123 expiry_time 2018-11-11 01:04:42.758849257 +0000 UTC lifetime_duration  max_duration  host studioml-go-runner-deployment-7c89df5878-ddd4w]
2018-11-07T01:05:09+0000 DBG runner stopping run _: [host studioml-go-runner-deployment-7c89df5878-ddd4w]
{"experiment":{"loss":0.2441,"acc":0.9298,"val_loss":0.2334,"val_acc":0.9318}}
```

# Implementation

The runner is used as a Go lang statically compiled alternative to the python runner.  It is intended to be used to run python based TensorFlow, and other GPU workloads using private cloud, and/or datacenter infrastructure with the experimenter controlling storage dependencies on public or cloud based infrastructure.

Using the runner with the open source StudioML tooling can be done without making changes to the python based StudioML.  Any configuration needed to use self hosted queues, or storage can be made using the StudioML yaml configuration file.  The runner is compatible with the StudioML completion service.

StudioML orchestrates the execution of jobs using two types of resources.  Firstly a message queue a used to submit python or containerized GPU workloads, for example TensorFlow tasks, that StudioML compliant runners can retrieve and process.  Secondly StudioML stores artifacts, namely files, using a storage service.

A reference deployment for the runner uses rabbitMQ for queuing, and minio for storage using the S3 v4 http protocol.  The reference deployment is defined to allow for portability across cloud, and data center infrstructure and for testing.

StudioML also supports hosted queues offered by cloud providers, namely AWS and Google cloud.  The storage features of StudioML are compatible with both cloud providers, and privately hosted storage services using the AWS S3 V4 API.

This present runner is capable of supporting several additional features beyond that of the StudioML python runner:

1. privately hosted S3 compatible storage services such as minio.io
2. static compute instances that provision GPUs that are shared across multiple StudioML experiments
3. Kubernetes deployments of runners to service the needs of StudioML users
3. (future) Allow runners to interact with StudioML API servers to retrieve metadata related to StudioML projects

# Using releases

The runner primary release vehicle is Github.  You will find a statically linked amd64 binary executable on Github.  This exectable can be packaged into docker containers for those wishing to roll their own solutions integration.

Several yaml and JSON files do exist within the examples directory that could be used as the basis for mass, or custom deployments.

Packaged releases are also available as Docker images from https://hub.docker.com/repository/docker/leafai/studio-go-runner, and https://quay.io/repository/leafai/studio-go-runner?tab=tags.  If you are using the Leaf AI platform pre-packaged versions may also be provided by your integrator.

Deployment of the Kubernetes style configurations on various cloud providers is documented in the following documents, docs/aws_k8s.md, docs/k8s.md, docs/aws_ecs_images.md, docs/azure.md, and docs/workstation_k8s.md.

# Using the code

The github repository should be a git clone of the https://github.com/studioml/studio.git repo.  Within the studio directories create a sub directory src and set your GOPATH to point at the top level studio directory.

```
mkdir ~/project
cd ~/project
export GOPATH=`pwd`
export PATH=$GOPATH/bin:$PATH
mkdir -p src/github.com/leaf-ai
cd src/github.com/leaf-ai
git clone https://github.com/leaf-ai/studio-go-runner.git
cd studio-go-runner
```

Code can be executed in one of two ways via docker based builds (please see the compilation section), or using the 'go build' command.

go runner builds are envisioned as occuring in two stages, the first being a local workstation shell, or docker based build and having an optional GPU.  The second stage being a build done using the CI/CD pipeline done using a Kubernetes cluster containing at least one GPU host, this is documented in the docs/ci.md file.

```
go run cmd/runner/main.go
```

# Compilation

This section describes builds performed using a developer workstation with or without docker, and an optional GPU.

## Prerequisties

### General Utilities

To deploy version managed CI/CD for the runner a version management tool is used to process the artifact files and to manage the docker containers within the system.

To install the tools on Ubuntu use the following commands:

```shell
mkdir -p $GOPATH/bin
go get github.com/karlmutch/petname
go install github.com/karlmutch/petname/cmd/petname
wget -O $GOPATH/bin/semver https://github.com/karlmutch/duat/releases/download/0.12.1/semver-linux-amd64
wget -O $GOPATH/bin/stencil https://github.com/karlmutch/duat/releases/download/0.12.1/stencil-linux-amd64
wget -O $GOPATH/bin/github-release https://github.com/karlmutch/duat/releases/download/0.12.1/github-release-linux-amd64
wget -O $GOPATH/bin/git-watch https://github.com/karlmutch/duat/releases/download/0.12.1/git-watch-linux-amd64
chmod +x $GOPATH/bin/semver
chmod +x $GOPATH/bin/stencil
chmod +x $GOPATH/bin/github-release
chmod +x $GOPATH/bin/git-watch
curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh
```

### Compilation Tools

This code based makes use of Go 1.11+.  The compiler can be found on the golang.org web site for downloading. On Ubuntu the following command can be used:

```
sudo apt-get install golang-1.11-go
```

go dep is used as the dependency management tool.  You do not need to use this tool except during active development. go dep software, and its installation instructions can be found at https://github.com/golang/dep.  go dep is intended to be absorbed into the go toolchain but for now can be obtained independently if needed.  All dependencies for this code base are checked into github following the best practice suggested at https://www.youtube.com/watch?v=eZwR8qr2BfI.

In addition to the go dep generated dependencies this software uses the CUDA development 8.0 libraries.
Releasing the service using versioning for Docker registries, or cloud provider registries requires first that the version for release is tagged with the desired version using the semver tool to first brand the README.md and other files and then to tag docker repositories.

In order to assist with builds and deploying the runner a Dockerfile\_developer is provided to allow for builds without extensive setup.  The Dockerfile requires Docker CE 17.06, or later, to build the runner.  The first command only needs to be run when the compilation tools, or CUDA version is updated, it is lengthy and typically takes 30 minutes but is only needed once.  The docker run command can be rerun everytime the source code changes quickly to perform builds.

The build tool will produce a list of binaries produced by the build that can be feed into tools such as github-release from duat.  The form of the docker run in the following example is what should be used when the release processing is not being done within the container.  The piped commands will correct the output file names printed for the environment outside of the container context.

```
dep ensure
stencil < Dockerfile_developer | docker build -t runner-build -
docker run -v $GOPATH:/project runner-build | sed 's/\/project\//$GOPATH\//g'| envsubst
```

If you are performing a release for a build using the containerize build then the GITHUB\_TOKEN environment must also be set in order for the github release to be pushed correctly.  In these cases the command line would appear as follows:

```
docker run -e GITHUB_TOKEN=$GITHUB_TOKEN -v $GOPATH:/project runner-build | sed 's/\/project\//$GOPATH\//g'| envsubst
```

After the container from the run completes you will find a runner binary file in the $GOPATH/src/github.com/leaf-ai/studio-go-runner/bin directory.

In order to create containerized version of the runner you will need to make use of the build.go tool and this requires that go 1.11 or later to be installed.  Ubuntu instructions can be found for go 1.11 at, https://github.com/golang/go/wiki/Ubuntu.To produce a tagged container for the runner use the following command, outside of container which will allow the containerization step to run automatically:

```
go run ./build.go -r
```

It is possible to produce containerized versions of the go runner when implementing the Kubernetes based pipeline documented in docs/ci.md.  You can also use the instructions in the CI guide to run your own copy of Ubers Makisu to produce containers using raw docker to do this rather than the pipeline is left to the reader.

# Running go runner  (Standalone)

The go runner has been designed to be adaptive to run in any type of deployment environment, cloud, on-premise for in VM infrastructure.  The following sections describe some reference deployment styles that are being used on a regular basis.  If you wish for a different deployment model please talk with a Sentient staff member for guidence.

## Non containerized deployments

When using Ubuntu the following GCC compilers and tools need to be installed to support the C++ and C code embeeded within the python machine learning frameworks being used:

```
sudo apt-get update
sudo apt-get install gcc-4.8 g++-4.8
sudo apt-get install libhdf5-dev liblapack-dev libstdc++6 libc6
```

StudioML uses the python virtual environment tools to deploy python applications and uses no isolation other than that offered by python.

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

python 2.7 and 3.5 must be installed as a prerequiste and a pip install should be done for the following wheel files:

```
sudo -H pip install -q pipenv Cython grpcio google-api-python-client google-cloud-storage google-cloud-pubsub google-cloud-core
sudo -H pip install -q https://storage.googleapis.com/tensorflow/linux/gpu/tensorflow_gpu-1.4.1-cp27-none-linux_x86_64.whl
sudo -H pip install -q scipy numpy scikit-learn h5py keras
sudo -H pip install -q http://download.pytorch.org/whl/cu80/torch-0.2.0.post3-cp27-cp27mu-manylinux1_x86_64.whl 
sudo -H pip install -q torchvision
sudo -H pip install -q pyopenssl --upgrade
```

The go based runner can make use of Singularity, a container platform, to provide isolation and also access to low level machine resources such as GPU cards.  This fuctionality is what differentiates the go based runner from the python based runners that are found within the open source StudioML offering.  Singlularity support is offered as an extension to the StudioML ecosystem however using its use while visible to StudioML affects it in no way.

Having completed the initial setup steps you should visit the https://github.com/leaf-ai/studio-go-runner/releases page and download the appropriate version of the runner and use it directly.

## Containerized deployments

The runner can be deployed using a container registry within cloud or on-premise environments.  The runner code comes bundled with a Dockerfile within the cmd/runner directory that can be used to generate your own images for deployment into custom solutions.

If you are using StudioML in conjunction with Sentient Technologies projects it is highly likely that a Container Registry has already been made available.  Talk with Sentient staff about the exact arrangements and your requirements for software deployment.

Containerized workloads can be orchestrated using Kubernetes.  The cloud based deployments for the go runner employ Kubernetes in order to maintain vendor neutrality and reduce support complexity.  If you wish to make use of vendor specific container orchestration frameworks, for example AWS FarGate, you will need to use the vendor specific tooling which while possible, does not fall within the scope of this document.

A containerized version of the runner can be created using the Dockerfile in the cmd/runner directory as follows:

```console
cd cmd/runner
export SEMVER=`semver -f ../../README.md`
docker build -t leafai/studio-go-runner:$SEMVER .
docker push leafai/studio-go-runner:$SEMVER
cd -
```


# Options and configuration

The runner supports command options being specified on the command line as well as by using environment variables.  Any command line option can be used within the environment variables by using all capitals and underscores in place of dashes.

## Cloud support

The primary deployment case for the runner uses using Kubernetes and containers.  As a result the cloud option associated with StudioML is typically not used except to identify the well known address of a queue server.  The rabbitMQ documentation contains notes concerning specific configuration for this section.  The default cloud settings should appear in your ~/.stdioml/config.yaml file as follows:

```
cloud:
    type: none
```

## Credentials management

The runner uses a credentials options, --certs-dir, to point at a directory location into which credentials for accessing cloud based queue and storage resources can be placed.  In order to manage the queues that runners will pull work from an orchestration system such as kubernetes, or salt should be used to manage the credentials files appearing in this directory.  Adding and removing credentials enables administration of which queues the runners on individual machines will be interacting with.

The existance of a credentials file will trigger the runner to list the queue subscriptions that are accessible to each credential and to then immediately begin pulling work from the same.

## AWS SQS and authentication

AWS queues can also be used to queue work for runners, regardless of the cloud that was used to deploy the runner.  The credentials in a data center or cloud environment will be stored using files within the container or orchestration run time.

The AWS credentials are deployed using files for each credential within the directory specified by the --sqs-certs option.  When using this sqs-certs option care should be taken to examine the default queue name filter option used by the runner, queue-match.  Typically this option will use a regular expression to only examine queues prefixed with either 'sqs\_' or 'rmq\_'.  Using the regular expression to include only a subset of queues can be used to partition work across queues that specific k8s clusters will visit to retrieve work.

When using Kubernetes AWS credentials are stored using the k8s cluster secrets feature and are mounted into the runner container.

## RabbitMQ access

RabbitMQ is supported by StudioML and the golang runner and an alternative to SQS.  To make use of rabbitMQ a url should be included in the studioML configuration file that details the message queue.  For example:

```
cloud:
    type: none
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

## Logging

The runner does support options for logging and monitoring.  For logging the logxi package options are available.  For example to print logging for debugging purposes the following variables could also be set in addition to the above example:

```
LOGXI_FORMAT=happy,maxcol=1024 LOGXI=*
```

## Slack reporting

The reporting of job results in slack can be done using the go runner.  The slack-hook option can be used to specify a hook URL, and the slack-room option can be used to specify the destination of tracking messages from the runner.

## Device Selection

The go runner supports CUDA\_VISIBLE\_DEVICES as a means by which the runner can be restricted to the use of specific GPUs within a machine.

Options CPU\_ONLY, MAX\_CORES, MAX\_MEM, MAX\_DISK and also be used to restrict the types and magnitude of jobs accepted.

# Data storage support

The runner supports both S3 V4 and Google Cloud storage platforms.  The StudioML client is responsible for passing credentials down to the runner using the StudioML configuration file.

Google storage allows for public, or private google cloud data to be used with the go runner with a single set of credentials.

A StudioML client yaml configuration file for google firebase storage can be specified like the following:

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

The S3 storage support can be used for runners that are either shared or are privately configured.  When using StudioML to submit work the experimenter can used the yaml configuration file to pass their local AWS configuration environment variables through to the runner using a file such as the following:

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

Copyright © 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
