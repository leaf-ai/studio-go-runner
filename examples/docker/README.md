# Docker Desktop multi runner deployment

This document discusses how to run a Docker Desktop deployment on a single Laptop or Desktop.

These instructions are intended for Mac or Windows experimenters.  For Linux please see the (Linux Kubernetes local example)[examples/local/README.md]

These instructions are generally intended for CPU users, however they can also apply to multiple GPUs within a single host if the [nvidia for docker tooling](https://github.com/NVIDIA/nvidia-docker) is installed.

The motivation behind this style of deployment of the runner is for cases where python based applications or frameworks and libraries they use are not capable of scaling beyond a single thread of execution, or are not thread-safe.

<!--ts-->

Table of Contents
=================

* [Docker Desktop multi runner deployment](#docker-desktop-multi-runner-deployment)
* [Table of Contents](#table-of-contents)
* [Introduction](#introduction)
* [Pre-requisites](#pre-requisites)
  * [Docker Desktop](#docker-desktop)
  * [Kubernetes CLI](#kubernetes-cli)
  * [Minio CLI](#minio-cli)
  * [Validation](#validation)
* [Configuration and Deployment](#configuration-and-deployment)
  * [Create storage service](#create-storage-service)
  * [Create the cluster](#create-the-cluster)
  * [Validation](#validation-1)
  * [A note on performance monitoring](#a-note-on-performance-monitoring)
* [Using the Cluster](#using-the-cluster)
  * [Starting experiments](#starting-experiments)
  * [Retrieving results](#retrieving-results)
<!--te-->

# Introduction

Using this document you will be able to run multiple studioml go runners on a single docker host.

# Pre-requisites

Before using the following instructions experimenters will need to have [Docker Desktop 2.3+ service installed](https://www.docker.com/products/docker-desktop).

This option requires at least 8Gb of memory in the minimal setups.

Any tools and servers used within the deployment are version controlled by the dockerhub container registry and so do not need to be specified.

## Docker Desktop

Once Docker Desktop is installed use the Windows Start-\>Docker menu, or Mac OSX menubar for Docker Desktop to perform the following actions :

* Use the Preferences Resources tab to increase the amount of RAM allocated to Docker to at least 8Gb.

* Activate the Kubernetes feature using the Preferences option in the menu. In addition the menu should show a green light and the "Kubernetes is running" indication inside the menu Kubernetes has initialized and is ready for use.  For more details please see, [https://docs.docker.com/desktop/](https://docs.docker.com/desktop/).

* Use the Kubernetes menu item to check that the Kubernetes instance installed and defaults to is the 'docker-desktop' instance.

* Export the kubectl configuration for your local cluster, see instructions in the validation section.

## Kubernetes CLI

kubectl can be installed using instructions found at:

- kubectl https://kubernetes.io/docs/tasks/tools/install-kubectl/

## Minio CLI

Minio offers a client for the file server inside the docker cluster called, [mc](https://docs.min.io/docs/minio-client-quickstart-guide.html).

The quickstart guide details installation for Windows, and Mac.  For Mac [Homebrew](https://brew.sh/) is used as shown:

```
brew install minio/stable/mc
```

## Validation

docker context export default --kubeconfig ~/.kube/docker.kubeconfig

To validate your installation you can now leave the KUBE\_CONFIG, and KUBECONFIG environment variables set, or set then to point at your exported configuration file '~/.kube/docker.kubeconfig', this will allow the kubectl tool to default to using your localhost to communicate with the cluster.

Now the kubectl command access can be tested as shown in the following Mac example:

```
$ kubectl get nodes
NAME             STATUS   ROLES    AGE     VERSION
docker-desktop   Ready    master   2m12s   v1.16.6-beta.0
$ kubectl describe nodes
Name:               docker-desktop
Roles:              master
Labels:             beta.kubernetes.io/arch=amd64
                    beta.kubernetes.io/os=linux
                    kubernetes.io/arch=amd64
                    kubernetes.io/hostname=docker-desktop
                    kubernetes.io/os=linux
                    node-role.kubernetes.io/master=
Annotations:        kubeadm.alpha.kubernetes.io/cri-socket: /var/run/dockershim.sock
                    node.alpha.kubernetes.io/ttl: 0
                    volumes.kubernetes.io/controller-managed-attach-detach: true
CreationTimestamp:  Mon, 04 May 2020 15:17:10 -0700
Taints:             <none>
Unschedulable:      false
Lease:
  HolderIdentity:  docker-desktop
  AcquireTime:     <unset>
  RenewTime:       Mon, 04 May 2020 16:17:12 -0700
Conditions:
  Type             Status  LastHeartbeatTime                 LastTransitionTime                Reason                       Message
  ----             ------  -----------------                 ------------------                ------                       -------
  MemoryPressure   False   Mon, 04 May 2020 16:16:23 -0700   Mon, 04 May 2020 15:17:08 -0700   KubeletHasSufficientMemory   kubelet has sufficient memory available
  DiskPressure     False   Mon, 04 May 2020 16:16:23 -0700   Mon, 04 May 2020 15:17:08 -0700   KubeletHasNoDiskPressure     kubelet has no disk pressure
  PIDPressure      False   Mon, 04 May 2020 16:16:23 -0700   Mon, 04 May 2020 15:17:08 -0700   KubeletHasSufficientPID      kubelet has sufficient PID available
  Ready            True    Mon, 04 May 2020 16:16:23 -0700   Mon, 04 May 2020 15:17:08 -0700   KubeletReady                 kubelet is posting ready status
Addresses:
  InternalIP:  192.168.65.3
  Hostname:    docker-desktop
Capacity:
  cpu:                6
  ephemeral-storage:  61255492Ki
  hugepages-1Gi:      0
  hugepages-2Mi:      0
  memory:             2038544Ki
  pods:               110
Allocatable:
  cpu:                6
  ephemeral-storage:  56453061334
  hugepages-1Gi:      0
  hugepages-2Mi:      0
  memory:             1936144Ki
  pods:               110
System Info:
  Machine ID:                 cff33312-1793-4201-829d-010a1525d327
  System UUID:                fb714256-0000-0000-a61c-ee3a89604c3a
  Boot ID:                    1d42a706-7f4f-4c91-8ec9-fd53bf1351bc
  Kernel Version:             4.19.76-linuxkit
  OS Image:                   Docker Desktop
  Operating System:           linux
  Architecture:               amd64
  Container Runtime Version:  docker://19.3.8
  Kubelet Version:            v1.16.6-beta.0
  Kube-Proxy Version:         v1.16.6-beta.0
Non-terminated Pods:          (11 in total)
  Namespace                   Name                                      CPU Requests  CPU Limits  Memory Requests  Memory Limits  AGE
  ---------                   ----                                      ------------  ----------  ---------------  -------------  ---
  docker                      compose-78f95d4f8c-6lp49                  0 (0%)        0 (0%)      0 (0%)           0 (0%)         58m
  docker                      compose-api-6ffb89dc58-qgnpq              0 (0%)        0 (0%)      0 (0%)           0 (0%)         58m
  kube-system                 coredns-5644d7b6d9-2xr4r                  100m (1%)     0 (0%)      70Mi (3%)        170Mi (8%)     59m
  kube-system                 coredns-5644d7b6d9-vvpzk                  100m (1%)     0 (0%)      70Mi (3%)        170Mi (8%)     59m
  kube-system                 etcd-docker-desktop                       0 (0%)        0 (0%)      0 (0%)           0 (0%)         58m
  kube-system                 kube-apiserver-docker-desktop             250m (4%)     0 (0%)      0 (0%)           0 (0%)         58m
  kube-system                 kube-controller-manager-docker-desktop    200m (3%)     0 (0%)      0 (0%)           0 (0%)         58m
  kube-system                 kube-proxy-tdsn2                          0 (0%)        0 (0%)      0 (0%)           0 (0%)         59m
  kube-system                 kube-scheduler-docker-desktop             100m (1%)     0 (0%)      0 (0%)           0 (0%)         58m
  kube-system                 storage-provisioner                       0 (0%)        0 (0%)      0 (0%)           0 (0%)         58m
  kube-system                 vpnkit-controller                         0 (0%)        0 (0%)      0 (0%)           0 (0%)         58m
Allocated resources:
  (Total limits may be over 100 percent, i.e., overcommitted.)
  Resource           Requests    Limits
  --------           --------    ------
  cpu                750m (12%)  0 (0%)
  memory             140Mi (7%)  340Mi (17%)
  ephemeral-storage  0 (0%)      0 (0%)
Events:
  Type    Reason                   Age                From                        Message
  ----    ------                   ----               ----                        -------
  Normal  Starting                 60m                kubelet, docker-desktop     Starting kubelet.
  Normal  NodeHasSufficientMemory  60m (x8 over 60m)  kubelet, docker-desktop     Node docker-desktop status is now: NodeHasSufficientMemory
  Normal  NodeHasNoDiskPressure    60m (x8 over 60m)  kubelet, docker-desktop     Node docker-desktop status is now: NodeHasNoDiskPressure
  Normal  NodeHasSufficientPID     60m (x7 over 60m)  kubelet, docker-desktop     Node docker-desktop status is now: NodeHasSufficientPID
  Normal  NodeAllocatableEnforced  60m                kubelet, docker-desktop     Updated Node Allocatable limit across pods
  Normal  Starting                 59m                kube-proxy, docker-desktop  Starting kube-proxy.
```

# Configuration and Deployment

## Create storage service

Minio is used to create a storage server for runner clusters when AWS is not being used.  This step will create a storage service with 10Gb of space.  It uses the persistent volume claim feature to retain any data the server has been sent and to prevent restarts from loosing the data.  The following steps are a summary of what is needed to standup the server:

```
kubectl create -f https://raw.githubusercontent.com/minio/minio/master/docs/orchestration/kubernetes/minio-standalone-pvc.yaml
kubectl create -f https://raw.githubusercontent.com/minio/minio/master/docs/orchestration/kubernetes/minio-standalone-deployment.yaml
kubectl create -f https://raw.githubusercontent.com/minio/minio/master/docs/orchestration/kubernetes/minio-standalone-service.yaml
```


More detailed information is available from [Minio Standalone Deployment](https://github.com/minio/minio/blob/master/docs/orchestration/kubernetes/k8s-yaml.md#minio-standalone-server-deployment).

## Create the cluster

To create the cluster a Kubernetes deployment yaml file is used and can be applied using applied using the 'kubectl -f [filename]' command. The deployment file can be obtained from this github project at [examples/docker/deployment.yaml](https://raw.githubusercontent.com/leaf-ai/studio-go-runner/master/examples/docker/deployment.yaml).

Before applying this file examine its contents and locate the studioml-go-runner-deployment deployment section, and then the resources subsection .  The resources subsection contains the hardware resources that will be assigned to the studioml runner pod.  Edit the resources to fit with your local machines capabilities and the resources needed to run your workloads.  The default 'replicas' value in the studioml-go-runner-deployment deployment section is set to 1 to reflect having a single runner.

The runner will divide the up the resources it has been allocated to service jobs arriving from your local 'studio run', or completion service.  As jobs are received by the runner the work will be apportioned by the runner and once the runner has allocated the resources that it has available it will stop secheduling more workers until sufficent resources are released.  On a single node there is no need to run more than one runner, expect in testing situations and the like where there might be a functional requirement.

You should also examine the cpu and memory sizings to ensure that the runner deployment pod fits and can be run by the cluster, if not they will remain in a 'Pending' state.  This can be done using the 'kubectl describe node' command and examining the hardware assigned to run the cluster.

Once you have checked the deployment file it can be applied as follows:

```
export KUBE_CONFIG=~/.kube/docker.kubeconfig
export KUBECONFIG=~/.kube/docker.kubeconfig
```

or

```
unset KUBE_CONFIG
unset KUBECONFIG
```

then

```
kubectl apply -f deployment.yaml
```

## Validation

Having created the services you can validate access to your freshly deployed services as shown in the following example:

```
$ kubectl get svc
NAME               TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)                          AGE
kubernetes         ClusterIP      10.96.0.1        <none>        443/TCP                          20h
minio-service      LoadBalancer   10.104.248.60    localhost     9000:30767/TCP                   10m
rabbitmq-service   LoadBalancer   10.104.168.157   localhost     15672:30790/TCP,5672:31312/TCP   2m22s
```


You will notice that the ports have been exposed to the localhost interface of your Mac or Windows machine.  This allows you to for example use your browser to access minio on 'http://localhost:9000', using a username of 'minio' and password of 'minio123'.  The rabbitMQ administration interface is on 'http://localhost:9000', username 'guest', and password 'guest'.

Clearly an insecure deployment intended just for testing, and benchmarking purposes.  If you wish to deploy these services with your own usernames and passwords examine the YAML files used for deployments and modify them with appropriate values for your situation.

For more information on exposing ports from Kubernetes please see, [accessing an application in Kubernetes](https://medium.com/@lizrice/accessing-an-application-on-kubernetes-in-docker-1054d46b64b1)

## A note on performance monitoring

There are two basic ways to get a sense of dynamic CPU and memory consumption.

* The first is to use 'docker stats'.  This is the simplest and probably best approach.

* The second is to use the Kubernetes Web UI dashboard, more details below.

If you wish to use dashboard style monitoring of your local clusters resource consumption you can use the Kubernetes Dashboard which has an introduction at [Web UI (Dashboard)](https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard/), and detailed access and installation instructions at, [https://github.com/kubernetes/dashboard](https://github.com/kubernetes/dashboard/blob/master/README.md).

# Using the Cluster

## Starting experiments

Having deployed the cluster we can now launch studio experiments using the localhost for our queue and for our storage.  To do this your studioml config.yaml file should be updated something like the following:

```
database:
    type: s3
    endpoint: http://minio-service.default.svc.cluster.local:9000
    bucket: metadata
    authentication: none

storage:
    type: s3
    endpoint: http://minio-service.default.svc.cluster.local:9000
    bucket: storage

cloud:
    queue:
        rmq: "amqp://guest:guest@rabbitmq-service.default.svc.cluster.local:5672/%2f?connection_attempts=30&retry_delay=.5&socket_timeout=5"

server:
    authentication: None

resources_needed:
    cpus: 1
    hdd: 10gb
    ram: 2gb

env:
    AWS_ACCESS_KEY_ID: minio
    AWS_SECRET_ACCESS_KEY: minio123
    AWS_DEFAULT_REGION: us-west-2i

verbose: debug
```

In order to access the minio and rabbitMQ servers the host names being used will need to match between the experiment host where experiments are launched and host names inside the compute cluster.  To do this the /etc/hosts, typically using 'sudo vim /etc/hosts', file of your local experiment host will need the following line added.

```
127.0.0.1 minio-service.default.svc.cluster.local rabbitmq-service.default.svc.cluster.local
```

If you wish you can use one of the examples provided by the StudioML python client to test your configuration, github.com/studioml/studio/examples/keras. Doing this will look like the following example:

```
cd studio/examples/keras
export AWS_ACCESS_KEY_ID=minio
export AWS_SECRET_ACCESS_KEY=minio123
studio run --lifetime=30m --max-duration=20m --gpus 0 --queue=rmq_kmutch --force-git train_mnist_keras.py
```

## Retrieving results

There are many ways that can be used to retrieve experiment results from the minio server.

The Minio Client (mc) mentioned as a prerequiste can be used to extract data from folders on the minio recursively as shown in the following example:

```
mc config host add docker-desktop http://minio-service.default.svc.cluster.local:9000 minio minio123
mc cp --recursive docker-desktop/storage/experiments experiment-results
```

It should be noted that the bucket names in the above example originate from the ~/.studioml/config.yaml file.

Additional information related to the minio client can be found at [MinIO Client Complete Guide](https://docs.min.io/docs/minio-client-complete-guide.html).

Copyright © 2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
