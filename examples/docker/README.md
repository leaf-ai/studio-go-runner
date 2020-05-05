# Docker Desktop multi runner deployment

This document discusses how to run a Docker Desktop deployment on a single Laptop or Desktop.

These instructions are intended for Mac or Windows experimenters.

These instructions are generally intended for CPU users, however they can also apply to multiple GPUs within a single host if the [nvidia for docker tooling](https://github.com/NVIDIA/nvidia-docker) is installed.

The motivation behind this style of deployment of the runner is for cases where python based applications or frameworks and libraries they use are not capable of scaling beyond a single thread of execution, or are not thread-safe.

<!--ts-->
<!--te-->

# Introduction

Using this document you will be able to run multiple studioml go runners on a single docker host.

# Pre-requisites

Before using the following instructions experimenters will need to have Docker Desktop 2.3+ service installed.

This option requires at least 8Gb of memory in the minimal setups.

## Docker Desktop

Once the Docker Desktop is installed use the Windows Start->Docker menu, or Mac OSX menubar for Docker Desktop to perform the following actions :


* Use the Preferences Resources tab to increase the amount of RAM allocated to Docker to at least 8Gb.

* Activate the Kubernetes feature using the Prefences option in the menu. In addition the menu should show a green light and the "Kubernetes is running" indication inside the menu Kubernetes has initialized and is ready for use.  For more details please see, [https://docs.docker.com/desktop/](https://docs.docker.com/desktop/).

* Use the Kubernetes menu item to check that the Kubernetes instance installed and defaults to is the 'docker-desktop' instance.

## Kubernetes CLI

kubectl can be installed using instructions found at:

- kubectl https://kubernetes.io/docs/tasks/tools/install-kubectl/

## Minio CLI

Minio offers a client for the file server inside the docker cluster called, [mc](https://docs.min.io/docs/minio-client-quickstart-guide.html).

The quickstart guide details installation for Windows and Mac.  For Mac [Homebrew](https://brew.sh/) is used as shown:

```
brew install minio/stable/mc
```

## Validation
To validate your installation ensure that the KUBE_CONFIG, and KUBECONFIG environment variables are not set, this will allow the kubectl tool to default to using your localhost to communicate with the cluster.

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

Retrieve the file examples/docker/deployment.yaml

Examine the memory sizings to ensure that the pods will all fit into memory.

kubectl apply deployment.yaml
