# Workstation and Laptop Kubernetes based testing software infrastructure

This document describes a laptop or local developer workstations testing environment for running leaf tests within a fully deployed studioML eco-system.

This document does not detail using GPU hardware in this setup primarily, however the microk8s tools do have support for a GPU plugin and this should work without major changes to the setup other than the GPU plugin installation.  The GPU support is only useful on PC equipment due to Mac OSX not supporting Nvidia hardware appropriate for TensorFlow.

## Introduction

In order to be able to run tests in a standalone format without depending upon publically deployed application infrastructure Kubernetes can be used to standup all of the components that StudioML requires.

In order to instantiate the servers needed Kubernetes is used to orchestrate multiple containers into a virtual private network.

The deployments needed include a Queue Server (RabbitMQ), a file server (minio using S3 V4), and the go runner (the system under test) to execute studioML experiments.

## Setup for single host Kubernetes

Single host Kubernetes deployments are typically what is used by an individual developer or for release based tasks where production cloud based clusters are not available.

For laptops, and private workstations using Windows 10 Professional Edition, or Mac OS 10.6 or later the infrastructure needs for Kubernetes can be meet by installation of Docker Desktop.  Once the docker desktop has been installed you can navigate to the Docker UI Preferences panel select the Kubernetes tab and then use a checkbox to install kubernetes.  Once this is done the machine will have a fully functional Kubernetes deployment that the testing instruction in this document details.

For Ubuntu hosts a microk8s solution exists that implements a single host deployment, https://microk8s.io/. Use snap on Ubuntu to install this component to allow for management of the optional features of microk8s.

The following example details how to configure microk8s once it has been installed:

```
# Allow the containers within the cluster to communicate with the public internet.  Needed for rabbitMQ pkg to be fetched and installed
sudo ufw default allow routed
sudo iptables -P FORWARD ACCEPT
sudo /snap/bin/microk8s.start
sudo /snap/bin/microk8s.enable dashboard dns ingress storage registry gpu
```

## Usage

Before following these instruction you will need to install the version management and template tools using the main README.md file, refer to the compilation section, and the prerequistes subsection.

### Docker based build

A prerequiste for following these instructions is that a local copy of the go runner has been checked out of github.  To this use the following commands:

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

Having obtained a copy of the studio go runner code the next step is to build an appropriate image for use in testing within a local single host kubernetes cluster.  If you make changes to source code you should re-build the image to refresh the contents with the new code.

```
cd ~/projects/src/github.com/leaf-ai/studio-go-runner
docker pull quay.io/leafai/studio-go-runner-dev-base:0.0.4
docker build -t leafai/studio-go-runner-standalone-build -f Dockerfile_standalone .
```

### Kubernetes test deployment and results collection

```
cd ~/projects/src/github.com/leaf-ai/studio-go-runner
export GIT_BRANCH=`echo '{{.duat.gitBranch}}'|stencil - | tr '_' '-' | tr '\/' '-'`
docker tag leafai/studio-go-runner-standalone-build:${GIT\_BRANCH} localhost:32000/leafai/studio-go-runner-standalone-build
docker push localhost:32000/leafai/studio-go-runner-standalone-build
/snap/bin/microk8s.kubectl apply -f test_k8s_local.yaml
/snap/bin/microk8s.kubectl --namespace build-test-k8s-local get pods
# Get the full pod name for the build-xxx pod and substitute it into the following command
# to get a full log of the test
/snap/bin/microk8s.kubectl --namespace build-test-k8s-local logs build-xxx
```

A kubernetes config file can be obtained from microk8s by using the following command:

```
/snap/bin/microk8s.kubectl config view --raw > ~/.kube/temp.config
export KUBE_CONFIG=~/.kube/temp.config
```

Copyright © 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
