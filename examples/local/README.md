# Quick introduction to using a locally deployed microk8s Kubernetes configuration

This document gives the briefest of overviews for standing up a single CPU runner cluster, with optional encryption support on a microk8s installation.

The microk8s use-case this document describes is somewhat similiar in its target audience to the (docker oriented deployment)[../docker/README.md] use case with the exception of our use of Kubernetes rather than docker itself.

The motivation behind this example is to provide from a deployment perspective something as close as possible on a single machine, or laptop to a real production deployment.  The docker use case is one that fits use cases needing a functionally equivalent deployment which can be done for Mac or Windows as a minimal alternative to a Linux deployment.

<!--ts-->

Table of Contents
=================

* [Quick introduction to using a locally deployed microk8s Kubernetes configuration](#quick-introduction-to-using-a-locally-deployed-microk8s-kubernetes-configuration)
* [Table of Contents](#table-of-contents)
* [Prerequisties](#prerequisties)
  * [Docker and Microk8s](#docker-and-microk8s)
  * [Deployment Tooling](#deployment-tooling)
  * [Kubernetes CLI](#kubernetes-cli)
  * [Minio CLI](#minio-cli)
  * [Steps](#steps)
    * [Deployment](#deployment)
    * [Running jobs](#running-jobs)
    * [Retrieving results](#retrieving-results)
    * [Cleanup](#cleanup)
<!--te-->

# Prerequisties

Before using the following instructions experimenters will need to have [Docker Desktop 2.3+ service installed](https://www.docker.com/products/docker-desktop).

This option requires at least 8Gb of memory in the minimal setups.

## Docker and Microk8s

You will need to install docker, and microk8s using Ubuntu snap.  When using docker installs only the snap distribution for docker is compatible with the microk8s deployment.

```console
sudo snap install docker --classic
sudo snap install microk8s --classic
```
When using microk8s during development builds the setup involved simply setting up the services that you to run under microk8s to support a docker registry and also to enable any GPU resources you have present to aid in testing.

```console
export LOGXI='*=DBG'
export LOGXI_FORMAT='happy,maxcol=1024'

export SNAP=/snap
export PATH=$SNAP/bin:$PATH

export KUBE_CONFIG=~/.kube/microk8s.config
export KUBECONFIG=~/.kube/microk8s.config

microk8s.stop
microk8s.start
microk8s.config > $KUBECONFIG
microk8s.enable registry:size=30Gi storage dns gpu
```

Now we need to perform some customization, the first step then is to locate the IP address for the host that can be used and then define an environment variable to reference the registry.  

```console
export RegistryIP=`microk8s.kubectl --namespace container-registry get pod --selector=app=registry -o jsonpath="{.items[*].status.hostIP}"`
export RegistryPort=32000
echo $RegistryIP
172.31.39.52
```

Now we have an IP Address for our unsecured microk8s registry we need to add it to the containerd configuration file being used by microk8s to mark this specific endpoint as being permitted for use with HTTP rather than HTTPS, as follows:

```console
sudo vim /var/snap/microk8s/current/args/containerd-template.toml
```

And add the last two lines in the following example to the file substituting in the IP Address we selected

```console
    [plugins.cri.registry]
      [plugins.cri.registry.mirrors]
        [plugins.cri.registry.mirrors."docker.io"]
          endpoint = ["https://registry-1.docker.io"]
        [plugins.cri.registry.mirrors."local.insecure-registry.io"]
          endpoint = ["http://localhost:32000"]
        [plugins.cri.registry.mirrors."172.31.39.52:32000"]
          endpoint = ["http://172.31.39.52:32000"]
```

```console
sudo vim /var/snap/docker/current/config/daemon.json
```

And add the insecure-registries line in the following example to the file substituting in the IP Address we obtained from the $RegistryIP

```console
{
    "log-level":        "error",
    "storage-driver":   "overlay2",
    "insecure-registries" : ["172.31.39.52:32000"]
}
```

The services then need restarting, note that the image registry will be cleared of any existing images in this step:

```console
microk8s.disable registry
microk8s.stop
sudo snap disable docker
sudo snap enable docker
microk8s.start
microk8s.enable registry:size=30Gi
```

You now have a registry which you can with your requirements in mind tag your own studio-go-runner images for and push to the registry in your local cluster using a command such as the following:

```
docker tag leafai/studio-go-runner:0.9.27 $RegistryIP:32000/leafai/studio-go-runner:0.9.27
docker push $RegistryIP:32000/leafai/studio-go-runner:0.9.27
```

## Deployment Tooling

Install a template processor based on the Go lang templater used by Kubernetes.

```
wget -O stencil https://github.com/karlmutch/duat/releases/download/0.13.0/stencil-linux-amd64
chmod +x stencil
```

## Kubernetes CLI

kubectl can be installed using instructions found at:

- kubectl https://kubernetes.io/docs/tasks/tools/install-kubectl/

## Minio CLI

Minio offers a client for the file server inside the docker cluster called, [mc](https://docs.min.io/docs/minio-client-quickstart-guide.html).

The quickstart guide details installation for Linux using a wget download as follows:

```
wget https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc
```

## Steps

These steps describe in summary form the actions needed to both initialize and access your locally deployed Kubernetes cluster.

### Deployment

Deployment uses the standard kubectl apply to instantiate all of the resources needed to have a fully functioning cluster.  The stencil command is used to fill in details such as the name of the docker image that is to be used along with its registra host and optional parameters such as a namespace dedicated to the deployed cluster.  Using a namespace is useful as it allows the go runner cluster to be isolated from other workloads.

The default cluster name if one is not supplied is local-go-runner.

```
stencil -input deployment.yaml -values Image=leafai/studio-go-runner:0.9.27 | kubectl apply -f -
```

After deployment there will be three pods inside the namespace and you will also have two services defined, for example:

```
$ kubectl --namespace local-go-runner get pods
NAME                                             READY   STATUS    RESTARTS   AGE
minio-deployment-7954bdbdc9-7w55b                1/1     Running   0          25m
rabbitmq-controller-6mkq6                        1/1     Running   0          25m
studioml-go-runner-deployment-5bddbccc94-54tq9   1/1     Running   0          25m
```

In order to view the logs of the various components the following commands might serve useful:

```
kubectl logs --namespace local-go-runner -f --selector=app=studioml-go-runner
...
kubectl logs --namespace local-go-runner -f --selector=app=minio
...
kubectl logs --namespace local-go-runner -f --selector=component=rabbitmq
...
```

### Running jobs

Before running a studioml job the configuration file should be populated as follows:

```
mkdir ~/.studioml
stencil -input studioml.config > ~/.studioml/config.yaml
```

Now a virtual environment can be created, studioml installed and a simple example run.

```
```

### Retrieving results

### Cleanup

```
kubectl delete namespace local-go-runner
```

Copyright © 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
