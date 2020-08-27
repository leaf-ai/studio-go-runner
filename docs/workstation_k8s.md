# Workstation and Laptop Kubernetes based testing software infrastructure

This document describes a laptop or local developer workstations testing environment for running leaf tests within a fully deployed studioML eco-system.

This document does not detail using GPU hardware in this setup, however the microk8s tools do have support for a GPU plugin and this should work without major changes to the setup other than the GPU plugin installation.  The GPU support is only useful on PC equipment due to Mac OSX not supporting Nvidia hardware appropriate for TensorFlow.

<!--ts-->
<!--te-->

## Introduction

In order to be able to run tests in a standalone format without depending upon publically deployed application infrastructure Kubernetes can be used to standup all of the components that StudioML requires.  In order to instantiate the servers needed Kubernetes is used to orchestrate multiple containers into a virtual private network.

A second use-case for quick turn arounds of testing cycles is to use locally deployed servers for the intermediate level developer.

The deployments needed include a Queue Server (RabbitMQ), a file server (minio using S3 V4), and the go runner (the system under test) to execute studioML experiments.

## Common build instructions

This section details the docker hosted build step. Prior to running this step docker should be installed on your development system.  For Windows and OSX you should use the Docker Desktop offering.

Also, you will need to install the version management and template tools using the main README.md file, refer to the compilation section, and the prerequistes subsection.

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
docker pull quay.io/leafai/studio-go-runner-dev-base:0.0.5
docker build -t leafai/studio-go-runner-standalone-build -f Dockerfile_standalone .
```

## Test and runtime instructions

### Single host Kubernetes (Incomplete)

Single host Kubernetes deployments are typically what is used by an individual developer or for release based tasks where production cloud based clusters are not available.

For laptops, and private workstations using Windows 10 Professional Edition, or Mac OS 10.6 or later the infrastructure needs for Kubernetes can be meet by installation of Docker Desktop.  Once the docker desktop has been installed you can navigate to the Docker UI Preferences panel select the Kubernetes tab and then use a checkbox to install kubernetes.  Once this is done the machine will have a fully functional Kubernetes deployment that the testing instruction in this document details.

For Ubuntu hosts a microk8s solution exists that implements a single host deployment, https://microk8s.io/. Use snap on Ubuntu to install this component along with Docker to allow for management of the optional features of microk8s.

The following example details how to configure Ubuntu microk8s once it has been installed:

```
# Allow the containers within the cluster to communicate with the public internet.  Needed for rabbitMQ pkg to be fetched and installed
sudo ufw default allow routed
sudo iptables -P FORWARD ACCEPT
sudo /snap/bin/microk8s.start
sudo /snap/bin/microk8s.enable dashboard dns ingress storage registry gpu
```


#### Kubernetes test deployment and results collection

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

### Single development host

#### Setup for a single host lightweight testing environment

These instruction give some guidance on using locally deployed go runner testing and is intended for development situations.

In order to run the services will need to be installed on your local host including RabbitMQ and Minio.

For minio please see the instructions found at, https://min.io/download#/linux.  Select the instructions for X64, I have not documented the docker version here but it can also be used with some modifications to point at the docker host.

The minio tests run with fixed user name (UserUser) and password (PasswordPassword). When deploying any test infrastructure you MUST not expose it to the outside world as it is intended to be emphemeral and is vulnerable due to intentionally open secrets.

Installing and starting minio could appear as follows:

```
wget https://dl.min.io/server/minio/release/linux-amd64/minio
chmod +x minio
mkdir -p /tmp/minio-data
MINIO_ACCESS_KEY=UserUser MINIO_SECRET_KEY=PasswordPassword ./minio server /tmp/minio-data
```

After testing be sure to bring down the process running the server and delete any data left in the /tmp/minio-data directory by the tests.

The second server, RabbitMQ, installation instructions can be found at, https://www.rabbitmq.com/download.html.  You can also use Docker for this but again this is not documented here.

Here is an example of using the bintray installation:

```
sudo apt-get update -y

## Install prerequisites
sudo apt-get install curl gnupg -y

## Install RabbitMQ signing key
curl -fsSL https://github.com/rabbitmq/signing-keys/releases/download/2.0/rabbitmq-release-signing-key.asc | sudo apt-key add -

## Install apt HTTPS transport
sudo apt-get install apt-transport-https

## Add Bintray repositories that provision latest RabbitMQ and Erlang 23.x releases
sudo tee /etc/apt/sources.list.d/bintray.rabbitmq.list <<EOF
## Installs the latest Erlang 23.x release.
## Change component to "erlang-22.x" to install the latest 22.x version.
## "bionic" as distribution name should work for any later Ubuntu or Debian release.
## See the release to distribution mapping table in RabbitMQ doc guides to learn more.
deb https://dl.bintray.com/rabbitmq-erlang/debian bionic erlang
## Installs latest RabbitMQ release
deb https://dl.bintray.com/rabbitmq/debian bionic main
EOF

## Update package indices
sudo apt-get update -y

## Install rabbitmq-server and its dependencies
sudo apt-get install rabbitmq-server -y --fix-missing

sudo rabbitmq-plugins enable rabbitmq_management

sudo su
cat <<EOF >>/etc/rabbitmq/rabbitmq.conf
# IPv4
listeners.tcp.local    = 127.0.0.1:5672
# IPv6
listeners.tcp.local_v6 = ::1:5672

management.tcp.ip   = 127.0.0.1
EOF
exit
```

Starting the service is done as follows:

```
service rabbitmq-server start
```

The default username (guest) and password (guest) can be used but again these are default and not secure so do not expose the server to the outside world.  By default these credentials can only be used on the localhost interface but credentials introduced by tests might also be a risk.

You are now ready to run tests. The following shows an example fo running a single internal test:

```
cd internal/runner
go test -v -tags=NO_CUDA -run TestSignatureBase
=== RUN   TestSignatureBase
--- PASS: TestSignatureBase (0.00s)
PASS
ok      github.com/leaf-ai/studio-go-runner/internal/runner     0.022s
cd -
```

The tests in the cmd/runner directory make use of the infrastructure and require additional options as shown in the following examples.

The following test is written to use its own minio that gets stood up for the duration of the test.

```
cd cmd/runner
# Create the base directories used by the server for its own private keys etc
mkdir -p ./certs/message/passphrase
mkdir -p ./certs/message/encryption
mkdir -p ./certs/queues/signing
mkdir -p ./certs/queues/response-encrypt
mkdir /tmp/cache

# Generate test keys for use by the server
echo -n "PassPhrase" > ./certs/message/passphrase/ssh-passphrase
ssh-keygen -t rsa -b 4096 -f ./certs/message/encryption/ssh-privatekey -C "Message Encryption Key" -N "PassPhrase"
chmod 0600 ./certs/message/encryption/ssh-privatekey
ssh-keygen -f ./certs/message/encryption/ssh-privatekey.pub -e -m PEM > ./certs/message/encryption/ssh-publickey
rm ./certs/message/encryption/ssh-privatekey.pub

export CLEAR_TEXT_MESSAGES=true
export AMQP_URL=amqp://guest:guest@127.0.0.1:5672/
go test -v -tags=NO_CUDA -a -cache-dir=/tmp/cache -cache-size=1Gib -test.timeout=10m -test.run=TestSignature
rm -rf /tmp/cache ./certs
cd -
```

Should you wish to cleanup any queue after the testing all of the queues on a server can be deleted using the following:

```
sudo rabbitmqctl --quiet list_queues name --no-table-headers | xargs -n 1 sudo rabbitmqctl delete_queue
```

The following shows an example of end to end test for CPU use-cases that is using a fully deployed style configuration:

```
go test -v -tags=NO_CUDA -a -cache-dir=/tmp/cache -cache-size=1Gib -test.timeout=30m -test.run=TestÄE2ECPUExperiment --use-k8s
```

Copyright © 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
