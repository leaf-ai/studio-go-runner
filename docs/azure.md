# Azure support for studio-go-runner

<script src='https://wzrd.in/standalone/copy-button@latest'></script>

This document describes the Azure specific steps for the installation and use of the studio-go-runner within Azure.

Before using these instruction you should have an Azure account and have full access to its service principal.  These instruction will guide you through the creation of a Kubernetes clusters using Microsoft specific tools.  After completing them you will be able to use the kubectl and other generic tools for installation of the go runner.

This Go, and the Python found within the reference implementation of StudioML, experiment runners have been tested on the Microsoft Azure cloud.

## Administration Prerequisites

TheAzure installation process will generate a number of keys and other valuable data during the creation of cloud based compute resources that will need to be sequestered in some manner.  In order to do this a long-lived host should be provisioned provisioned for use with the administration steps detailed within this document.


Azure can run Kubernetes as a platform for fleet management of machines and container orchestration using ace-engine, the preferred means of doing this, at least until AKS can support machine types that have GPU resources. kubectl can be installed using instructions found at:

- kubectl https://kubernetes.io/docs/tasks/tools/install-kubectl/

Docker is also used to manage images from an administration machine:

- Docker Ubuntu Installation, https://docs.docker.com/install/linux/docker-ce/ubuntu/#install-docker-engine---community

Instructions on getting started with the azure tooling needed for operating your resources can be found as follows:

- AZ CLI https://github.com/Azure/azure-cli#installation

If you are a developer wishing to push workloads to the Azure Container Service you can find more information at, https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-docker-cli.

## Installation


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

<copy-button>Test1</copy-button>
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
<copy-button>Test2</copy-button>
Once the main login has been completed you will be able to login to the container registry and other Azure services.  Container registries are named in the global namespace for Azure.

If you need to create a registry then the following commands will do this for you:

Switch to powershell or bash cloud shell
```shell
$ export azure_registry_name=leafai----USER NAME----
$ export registry_resource_group=studioml
$ export acr_principal=registry-acr-principal
$ az group create --name $registry_resource_group --location eastus
{
  "id": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/${registry_resource_group}",
  "location": "eastus",
  "managedBy": null,
  "name": "${registry_resource_group}",
  "properties": {
    "provisioningState": "Succeeded"
  },
  "tags": null
}
$ az acr create --name $azure_registry_name --resource-group $registry_resource_group --sku Basic
 - Running ..
```

Create a new service principal and assign access, this process will auto generate a password for the role:

```shell
registryId=$(az acr show --name $azure_registry_name --query id --output tsv)
registrySecret=$(az ad sp create-for-rbac --name http://$acr_principal --scopes $registryId --role acrpull --query password --output tsv)
registryAppId=$(az ad sp show --id http://$acr_principal --query appId --output tsv)
# Save the secret it is shown only ever once
$ az acr update -n $azure_registry_name --admin-enabled true
{
  "adminUserEnabled": true,
  "creationDate": "2018-02-15T19:10:18.466001+00:00",
  "id": "${registryId}",
  "location": "eastus",
  "loginServer": "${azure_registry_name}.azurecr.io",
  "name": "${azure_registry_name}",
  "provisioningState": "Succeeded",
  "resourceGroup": "${registry_resource_group}",
  "sku": {
    "name": "Basic",
    "tier": "Basic"
  },
  "status": null,
  "storageAccount": null,
  "tags": {},
  "type": "Microsoft.ContainerRegistry/registries"
}
```

```shell
$ az acr login --name $azure_registry_name
Login Succeeded
```

Resource groups are an organizing abstraction within Azure so when using the az command line tools you will need to be aware of the resource group you are operating within.

```
$ az acr list --resource-group $registry_resource_group --query "[].{acrLoginServer:loginServer}" --output table
AcrLoginServer
---------------------
${azure_registry_name}.azurecr.io
```

Pushing to Azure then becomes a process of tagging the image locally prior to the push to reflect the Azure login server, as follows:

```shell
$ docker pull leafai/studio-go-runner:0.9.21
0.9.21: Pulling from leafai/studio-go-runner
Digest: sha256:1f1776e678d139eb17bc138b7b06893b5b8c05b8bd95d7da13fef50636220c49
Status: Image is up to date for leafai/studio-go-runner:0.9.21
docker.io/leafai/studio-go-runner:0.9.21
$ docker tag leafai/studio-go-runner:0.9.21 $azure_registry_name.azurecr.io/${azure_registry_name}/studio-go-runner:0.9.21
$ docker push $azure_registry_name.azurecr.io/${azure_registry_name}/studio-go-runner:0.9.21
The push refers to repository [leafai.azurecr.io/leafai/studio-go-runner]
27b1d57fbfda: Pushed
cd8703242913: Pushed
c834dae8b4ec: Pushed
38b25d638c46: Pushed
16647619d7f5: Pushed
1d94fae18270: Pushed
a97d96ce1392: Pushed
374f7673dd29: Pushed
af43da66ca26: Pushed
7b3d60ae52a2: Pushed
c9f785c9f26f: Pushed
4b4d060df84e: Pushed
a66cc65b18b2: Pushed
296f7da2eaed: Pushed
8ff080c4dfcd: Pushed
c4adc63c1191: Pushed
22b28f4b003e: Pushed
f1dfa8049aa6: Pushed
79109c0f8a0b: Pushed
33db8ccd260b: Pushed
b8c891f0ffec: Pushed
0.9.21: digest: sha256:1f1776e678d139eb17bc138b7b06893b5b8c05b8bd95d7da13fef50636220c49 size: 4733
```

The go runner build pipeline will push images to Azure ACR when run in a shell that has logged into Azure and acr together.

Azure image repositories can be queried using the CLI tool, for example:

```shell
$ az acr repository show-tags --name $azure_registry_name --repository ${azure_registry_name}/studio-go-runner --output table
Result
--------------------
0.9.21
```

More information about the compatibility of the registry between Azure and docker hub can be found at, https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-docker-cli.

### Kubernetes and Azure

The aks-engine tool is used to create a Kubernetes cluster when hosting on Azure.  Within Azure, aks-engine acts much like kops does for AWS.  Like kops, aks-engine will read a template, see examples/azure/kubernetes.json, and will fill in the account related information and write the resulting Azure Resource Manager templates into the '\_output' directory.  The output directory will end up containing things such as SSH keys, k8s configuration files etc.  The kubeconfig files will be generated for each region the service can be deployed to, when using the kubectl tools set your KUBECONFIG environment variable to point at the desired region.  This will happen even if the region is specified using the --location command.

When handling multiple clusters the \_output directory will end up with multiple subdirectories, one for each cluster.  The directories are auto-generated and so you will need to keep track of their names and the clusters they apply to.  After using acs-engine deploy to generate and then deploy a cluster you should identify the directory that was created in your \_output area and then use that directory name in subsequent kubectl commands, when using the KUBECONFIG environment variable.

The example examples/azure/kubernetes.json file contains an empty Azure Client ID and secret.  Before running this command you will need to create a service principal and extract client ID and secret for it, updating this file in turn.  Those doing Azure account management and managing service principals might find the following helpful, https://github.com/Azure/aks-engine/blob/master/docs/topics/service-principals.md.

For information related to GPU workloads and k8s please review the following github page, https://github.com/Azure/aks-engine/blob/master/docs/topics/gpu.md.  Using his methodology means not having to be concerned about spining up the nivida plugins and the like.

The command lines show here are using the JMESPath query language for json which you can read about here, http://jmespath.org/.

```shell
az group create --name myResourceGroup --location eastus
az aks create --resource-group myResourceGroup --name myAKSCluster --node-vm-size Standard_NC6 --node-count 1
az aks get-credentials --resource-group myResourceGroup --name myAKSCluster
export KUBECONFIG=/home/kmutch/.kube/config
kubectl create namespace gpu-resources
kubectl apply -f examples/azure/nvidia-device-plugin-ds-1.11.yaml
kubectl create secret docker-registry studioml-go-docker-key --docker-server=$azure_registry_name.azurecr.io --docker-username=$registryAppId --docker-password=$registrySecret --docker-email=karlmutch@gmail.com
kubectl apply -f <(stencil < examples/azure/deployment-1.11.yaml)

$ k8s_resource_group=test-$USER-k8s
$ k8s_principal=${k8s_resource_group}-principal
$ az group create --name $k8s_resource_group --location eastus --query properties --output tsv
k8sSecret=$(az ad sp create-for-rbac --name http://$k8s_principal --role="Contributor" --scopes="/subscriptions/${subscription_id}/resourceGroups/${k8s_resource_group}" --query password --output tsv)
k8sAppId=$(az ad sp show --id http://$k8s_principal --query appId --output tsv)
$ az role assignment create --scope /subscriptions/${subscription_id}/resourceGroups/${registry_resource_group} --role AcrPull --assignee $k8sAppId
$ cp examples/azure/kubernetes.json example_k8s.json
$ aks-engine deploy --resource-group $k8s_resource_group --subscription-id $subscription_id --dns-prefix $k8s_resource_group --location eastus --auto-suffix --client-id=$k8sAppId --client-secret=$k8sSecret --api-model example_k8s.json
WARN[0002] apimodel: missing masterProfile.dnsPrefix will use "test-kmutch-k8s" 
INFO[0017] Starting ARM Deployment test-kmutch-k8s-550750629 in resource group test-kmutch-k8s. This will take some time... 
INFO[0368] Finished ARM Deployment (test-kmutch-k8s-550750629). Succeeded 
$ ls _output -alcrt
total 32
drwx------  3 kmutch kmutch 4096 Apr 26 15:52 test-kmutch-k8s-5ae2582e
drwx------  8 kmutch kmutch 4096 Apr 27 09:32 .
drwx------  3 kmutch kmutch 4096 Apr 27 09:33 test-kmutch-k8s-5ae350ba
$ k8s_prefix=test-kmutch-k8s-5ae350ba
$ export KUBECONFIG=_output/$k8s_prefix/kubeconfig/kubeconfig.eastus.json
$ kubectl get nodes
NAME                        STATUS    ROLES     AGE       VERSION
k8s-agentpool1-22074214-0   Ready     agent     11m       v1.9.7
k8s-master-22074214-0       Ready     master    11m       v1.9.7
```

### Azure Kubernetes Private Image Registry deployments

In order to access private image repositories k8s requires authenticated access to the repository.  In the following example we open access to the acr to the application created by the aks-engine.  The azurecr.io credentials can also be saved as k8s secrets as an alternative to using Azure service principals.  Using k8s secrets can be a little more error prone and opaque to the Azure platform so I tend to go with using Azure to do this.  If you do wish to go with the k8s centric approach you can find more information at, https://kubernetes.io/docs/concepts/containers/images/#using-azure-container-registry-acr.

To do the following you will need access to the service principal that operates the Client ID and secret you were given when creating the cluster.  The application ID can be found by examining the service principal in the Azure Web UI.  If you are using an account hosted by another party have them open access to the container registry you are intending to make use of.

```shell
$ az acr show --name $azure_registry_name
{
  "adminUserEnabled": true,
  "creationDate": "2018-02-12T22:13:48.208147+00:00",
  "id": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/${registry_resource_group}/providers/Microsoft.ContainerRegistry/registries/${azure_registry_name}",
  "location": "eastus",
  "loginServer": "${azure_registry_name}.azurecr.io",
  "name": "${azure_registry_name}",
  "provisioningState": "Succeeded",
  "resourceGroup": "${registry_resource_group}",
  "sku": {
    "name": "Basic",
    "tier": "Basic"
  },
  "status": null,
  "storageAccount": null,
  "tags": {},
  "type": "Microsoft.ContainerRegistry/registries"
}
$ acr_id=`az acr show --name $azure_registry_name --query "[id]" --out tsv`
$ az role assignment create --scope ${acr_id} --role AcrPull --assignee $k8sAppId
{
  "id": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/${registry_resource_group}/providers/Microsoft.ContainerRegistry/registries/${azure_registry_name}/providers/Microsoft.Authorization/roleAssignments/0397aa24-33b4-4bd7-957b-7a51cbe39570",
  "name": "0397aa24-33b4-4bd7-957b-7a51cbe39570",
  "properties": {
    "additionalProperties": {
      "createdBy": null,
      "createdOn": "2018-02-15T20:21:54.1315530Z",
      "updatedBy": "d31ae941-4fb9-4a82-bd53-a9471fbb2025",
      "updatedOn": "2018-02-15T20:21:54.1315530Z"
    },
    "principalId": "99999999-pppp-pppp-pppp-pppppppppppp",
    "roleDefinitionId": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/providers/Microsoft.Authorization/roleDefinitions/rrrrrrrr-rrrr-rrrr-rrrr-rrrrrrrrrrrr",
    "scope": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/${registry_resource_group}/providers/Microsoft.ContainerRegistry/registries/${azure_registry_name}"
  },
  "resourceGroup": "${registry_resource_group}",
  "type": "Microsoft.Authorization/roleAssignments"
}
```

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

The Minio server acts as the file distribution point for data processed by experiments.  This server is typically provisioned with the Azure resource group, but not the Kubernetes cluster.  The entry point machine type is recommended to be D4s\_v3, $163.68 per month.

minio software is downloaded from dockerhub, the image is named minio/minio, https://hub.docker.com/layers/minio/minio/RELEASE.2019-09-11T19-53-16Z/images/sha256-e6f79a159813cb01777eefa633f4905c1d4bfe091f4d40de317a506e1d10f30c.  Again quay.io is recommended for CVE scanning if desired.

Workers
-------

Kubernetes AKS Images and deployment details

AKS Base Image Distro w/ Ubuntu 18.04, April 2019

Workers, South Central Region, availability currently limited to NC6, NC12, NV6, NV12 $700-$1,600 per month

Software deployed to the worker is the studio-go-runner.  This software is available as open source and is provided also from the quay.io site.  As of 9.20.0, sha256:...aec406105f91 there are no high-level vulnerabilities.  This image can be pulled independently using, 'docker pull quay.io/leafai/studio-go-runner', the canonical URL is https://quay.io/repository/leafai/studio-go-runner/manifest/sha256:aec406105f917e150265442cb45794c67df0f8ee59450eb79cd904f09ded18d6.

Security Note
-------------

The Docker images being used within the solution are recommended, in high security situations, to be scanned independently for CVE's.  A number of services are available for this purposes including quay.io that can be used as this is not provided by the open source studio.ml project.  Suitable plans for managing enough docker repositories to deal with Studio.ML deployments typically cost in the $30 per month range from Quay.io, now Redhat Quay.io.

It is recommended that images intended for use within secured environments are first transferred into the Azure environment by performing docker pull operations from their original sources and then using docker tag, docker login, docker push operations then get transferred into the secured private registry of the Azure account holder.  This is recommended to prevent tampering with images after scanning is performed and also to prevent version drift.

Software Manifest
-----------------

The runner is audited on a regular basis for Open Source compliance using SPDX tools.  A total of 133 software packages are incorporated into the runner and are subject to source level security checking and alerting using github.  The manifest file for this purpose is produced during builds and can be provided by request.

More information abouth the source scanning feature can be found at, https://help.github.com/en/articles/about-security-alerts-for-vulnerable-dependencies.
