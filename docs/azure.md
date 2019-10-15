# Azure support for studio-go-runner

This document describes the Azure specific steps for the installation and use of the studio-go-runner within Azure.

Before using these instruction you should have an Azure account and have full access to its service principal.  These instruction will guide you through the creation of a Kubernetes clusters using Microsoft specific tools.  After completing them you will be able to use the kubectl and other generic tools for installation of the go runner.

This Go, and the Python found within the reference implementation of StudioML, experiment runners have been tested on the Microsoft Azure cloud.

## Administration Prerequisites

The Azure installation process will generate a number of keys and other valuable data during the creation of cloud based compute resources that will need to be sequestered in some manner.  In order to do this a long-lived host should be provisioned provisioned for use with the administration steps detailed within this document.

Your linux account should have an ssh key generated, see ssh-keygen man pages

Azure can run Kubernetes as a platform for fleet management of machines and container orchestration using AKS supporting regions with machine types that have GPU resources. kubectl can be installed using instructions found at:

- kubectl https://kubernetes.io/docs/tasks/tools/install-kubectl/

Docker is also used to manage images from an administration machine:

- Docker Ubuntu Installation, https://docs.docker.com/install/linux/docker-ce/ubuntu/#install-docker-engine---community

Instructions on getting started with the azure tooling needed for operating your resources can be found as follows:

- AZ CLI https://github.com/Azure/azure-cli#installation

If you are a developer wishing to push workloads to the Azure Container Service you can find more information at, https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-docker-cli.

## Installation Prerequisites

If Azure is being used then an Azure account will need and you need to authenticate with the account using the 'az login' command.  This will also require access to a browser to complete the login:

```shell
$ az login
To sign in, use a web browser to open the page https://aka.ms/devicelogin and enter the code B.......D to authenticate.
```

You will now need to determine the Azure subscription id that will be used for all resources that are consumed within Azure.  The current subscription ids available to you can be seen inside the Azure web portal or using the cmd line.  Take care to choose the appropriate license.  If you know you are using a default license then you can use the following command to save the subscription as a shell variable:

```shell
$ subscription_id=`az account list -otsv --query '[?isDefault].{subscriptionId: id}'`
```

If you have an Azure account with multiple subscriptions or you wish to change the default subscription you can use the az command to do so, for example:

```shell
$ az account list -otsv --all
AzureCloud      ...    True   Visual Studio Ultimate with MSDN        Enabled ...
AzureCloud      ...    False   Pay-As-You-Go   Warned  ...
AzureCloud      ...    False    Sentient AI Evaluation  Enabled ...
$ az account set --subscription "Sentient AI Evaluation"
$ az account list -otsv --all
AzureCloud      ...    False   Visual Studio Ultimate with MSDN        Enabled ...
AzureCloud      ...    False   Pay-As-You-Go   Warned  ...
AzureCloud      ...    True    Sentient AI Evaluation  Enabled ...
```

Once the subscription ID is selected the next step is to generate for ourselves an identifier for use with Azure resource groups etc that identifies the current userand local host to prevent collisions.  This can be done using rthe following commands:

```shell
uniq_id=`md5sum <(echo $subscription_id $(ip maddress show eth0)) |  cut -f1 -d\  | cut -c1-8`
````

## Compute cluster deployment

Once the main login has been completed you will be able to login to the container registry and other Azure services.  Be aware that container registries are named in the global namespace for Azure.

If you need to create a registry then the following commands will do this for you:

```shell
export azure_registry_name=leafai$uniq_id
export registry_resource_group=studioml-$uniq_id
export acr_principal=registry-acr-principal-$uniq_id
az group create --name $registry_resource_group --location eastus
az acr create --name $azure_registry_name --resource-group $registry_resource_group --sku Basic
```

Create a new service principal and assign access, this process will auto generate a password for the role.  The secret that is generated is only ever output once so a safe location should be found for it and it should be saved:

```shell
registryId=$(az acr show --name $azure_registry_name --query id --output tsv)
registrySecret=$(az ad sp create-for-rbac --name http://$acr_principal --scopes $registryId --role acrpull --query password --output tsv)
registryAppId=$(az ad sp show --id http://$acr_principal --query appId --output tsv)
az acr update -n $azure_registry_name --admin-enabled true
```

```shell
az acr login --name $azure_registry_name
Login Succeeded
```

Resource groups are an organizing abstraction within Azure so when using the az command line tools you will need to be aware of the resource group you are operating within.

```
az acr list --resource-group $registry_resource_group --query "[].{acrLoginServer:loginServer}" --output table
```

Pushing to Azure then becomes a process of tagging the image locally prior to the push to reflect the Azure login server, as follows:

```shell
docker pull leafai/studio-go-runner:0.9.21
docker tag leafai/studio-go-runner:0.9.21 $azure_registry_name.azurecr.io/${azure_registry_name}/studio-go-runner:0.9.21
docker push $azure_registry_name.azurecr.io/${azure_registry_name}/studio-go-runner:0.9.21
```

The go runner build pipeline will push images to Azure ACR when run in a shell that has logged into Azure and acr together.

Azure image repositories can be queried using the CLI tool, for example:

```shell
az acr repository show-tags --name $azure_registry_name --repository ${azure_registry_name}/studio-go-runner --output table
```

More information about the compatibility of the registry between Azure and docker hub can be found at, https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-docker-cli.

### Kubernetes and Azure

The az aks CLI tool is used to create a Kubernetes cluster when hosting on Azure, this command set acts much like kops does for AWS.  The following instructions will output a KUBECONFIG for downstream use by the Kubernetes tooling etc.  The kubeconfig files will be generated for each region the service can be deployed to, when using the kubectl tools set your KUBECONFIG environment variable to point at the desired region.  This will happen even if the region is specified using the --location command.

When handling multiple clusters the \_output directory will end up with multiple subdirectories, one for each cluster.  The directories are auto-generated and so you will need to keep track of their names and the clusters they apply to.  After using acs-engine deploy to generate and then deploy a cluster you should identify the directory that was created in your \_output area and then use that directory name in subsequent kubectl commands, when using the KUBECONFIG environment variable.

The example examples/azure/kubernetes.json file contains an empty Azure Client ID and secret.  Before running this command you will need to create a service principal and extract client ID and secret for it, updating this file in turn.  Those doing Azure account management and managing service principals might find the following helpful, https://github.com/Azure/aks-engine/blob/master/docs/topics/service-principals.md.

For information related to GPU workloads and k8s please review the following github page, https://github.com/Azure/aks-engine/blob/master/docs/topics/gpu.md.  Using his methodology means not having to be concerned about spining up the nivida plugins and the like.

The command lines show here are using the JMESPath query language for json which you can read about here, http://jmespath.org/.

```shell
export k8s_resource_group=leafai-$uniq_id
export aks_cluster_group=leafai-cluster-$uniq_id
az group create --name $k8s_resource_group --location eastus
az aks create --resource-group $k8s_resource_group --name $aks_cluster_group --node-vm-size Standard_NC6 --node-count 1
az aks get-credentials --resource-group $k8s_resource_group --name $aks_cluster_group
export KUBECONFIG=$HOME/.kube/config
kubectl create namespace gpu-resources
kubectl apply -f examples/azure/nvidia-device-plugin-ds-1.11.yaml
kubectl create secret docker-registry studioml-go-docker-key --docker-server=$azure_registry_name.azurecr.io --docker-username=$registryAppId --docker-password=$registrySecret --docker-email=karlmutch@gmail.com
kubectl apply -f <(stencil < examples/azure/deployment-1.11.yaml)
kubectl get pods
```

### Azure Kubernetes Private Image Registry deployments

In order to access private image repositories k8s requires authenticated access to the repository.  In the following example we open access to the acr to the application created by the aks-engine.  The azurecr.io credentials can also be saved as k8s secrets as an alternative to using Azure service principals.  Using k8s secrets can be a little more error prone and opaque to the Azure platform so I tend to go with using Azure to do this.  If you do wish to go with the k8s centric approach you can find more information at, https://kubernetes.io/docs/concepts/containers/images/#using-azure-container-registry-acr.

The following articles can shed more light on this process and provide a more detailed walkthrough of the alternatives.

https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/
https://thorsten-hans.com/how-to-use-a-private-azure-container-registry-with-kubernetes-9b86e67b93b6

If you wish to make use of kubernetes to store Azure registry access secrets then you would use a command such as the following:

```shell
kubectl create secret docker-registry studioml-go-docker-key --docker-server=$azure_registry_name.azurecr.io --docker-username=$registryAppId --docker-password=$registrySecret --docker-email=karlmutch@gmail.com
```

At this point the Kubernetes Pod template would need to be updated through the addition of the imagePullSecrets: section in the yaml.

```
apiVersion: v1
kind: Deployment
...
spec:
  containers:
    spec:
      serviceAccountName: studioml-account
      automountServiceAccountToken: false
      containers:
      - name: studioml-go-runner
...
      imagePullSecrets:
        - name: studioml-go-docker-key
```

A kubernetes cluster will now be installed and ready for the deployment of the studioml go runner.  To continue please return to the base installation instructions.

# Manifest and suggested deployment artifacts

Current studio-go-runner, aka runner, is recommended to be deployed within Azure using the components from the following as a starting point:

RabbitMQ Server
---------------

https://hub.docker.com/layers/rabbitmq/library/rabbitmq/3.7.17-alpine/images/sha256-bc92e61664e10cd6dc7a9bba3d39a18a446552f9dc40d2eb68c19818556c3201
OSI Compliant
quay.io with a micro plan can be used for CVE scanning

The RabbitMQ Server will be deployed within the Azure account and resource group but outside of the Kubernetes cluster.  The machine type is recommended to be DS12\_v2, $247 per month.

Minio S3 Server
---------------

The Minio server acts as the file distribution point for data processed by experiments.  The entry point machine type is recommended to be D4s\_v3, $163.68 per month.

minio software can downloaded from dockerhub, the image is named minio/minio.  Again quay.io is recommended for CVE scanning if desired.

Within Azure the minio server will typically be deployed using a standalone VM instance.  Using the Azure CLI a host should be stood up with a fixed IP address to ensure that the machine remains available after restarts.

https://docs.microsoft.com/en-us/azure/virtual-network/virtual-networks-static-private-ip-arm-cli

The minio server is installed on Ubuntu typically however any OS can be used, for example CentOS, https://www.centosblog.com/install-configure-minio-object-storage-server-centos-linux/
On Ubuntu the following instructions can be used, https://linuxhint.com/install_minio_ubuntu_1804/.

```shell
```

Workers
-------

Kubernetes AKS Images and deployment details

AKS Base Image Distro w/ Ubuntu 18.04, April 2019

Workers, East US Region, availability currently limited to NC6, NC12, NV6, NV12 $700-$1,600 per month

Software deployed to the worker is the studio-go-runner.  This software is available as open source and is provided also from the quay.io site.  As of 9.20.0, sha256:...aec406105f91 there are no high-level vulnerabilities.  This image can be pulled independently using, 'docker pull quay.io/leafai/studio-go-runner', the canonical URL is https://quay.io/repository/leafai/studio-go-runner/manifest/sha256:aec406105f917e150265442cb45794c67df0f8ee59450eb79cd904f09ded18d6.

Security Note
-------------

The Docker images being used within the solution are recommended, in high security situations, to be scanned independently for CVE's.  A number of services are available for this purposes including quay.io that can be used as this is not provided by the open source studio.ml project.  Suitable plans for managing enough docker repositories to deal with Studio.ML deployments typically cost in the $30 per month range from Quay.io, now Redhat Quay.io.

It is recommended that images intended for use within secured environments are first transferred into the Azure environment by performing docker pull operations from their original sources and then using docker tag, docker login, docker push operations then get transferred into the secured private registry of the Azure account holder.  This is recommended to prevent tampering with images after scanning is performed and also to prevent version drift.

Software Manifest
-----------------

The runner is audited on a regular basis for Open Source compliance using SPDX tools.  A total of 133 software packages are incorporated into the runner and are subject to source level security checking and alerting using github.  The manifest file for this purpose is produced during builds and can be provided by request.

More information abouth the source scanning feature can be found at, https://help.github.com/en/articles/about-security-alerts-for-vulnerable-dependencies.
