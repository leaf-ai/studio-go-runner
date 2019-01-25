# Azure support for studio-go-runner

This document describes the Azure specific steps for the installation and use of the studio-go-runner within Azure.

Before using these instruction you should have an Azure account and have full access to its service principal.  These instruction will guide you through the creation of a Kubernetes clusters using Microsoft specific tools.  After completing them you will be able to use the kubectl and other generic tools for installation of the go runner.

### Install Azure Cloud engine support (Azure only)

The Go and the Python runner found within the reference implementation of StudioML have been tested on the Microsoft Azure cloud.

Azure can run Kubernetes as a platform for fleet management of machines and container orchestration using ace-engine, the preferred means of doing this, at least until AKS can support machine types that have GPU resources.

Instructions on getting started with the azure tooling needed for operating your resources can be found as follows:

- AZ CLI https://github.com/Azure/azure-cli#installation
- acs-engine https://github.com/Azure/acs-engine/blob/master/docs/acsengine.md#install-acs-engine

If you are a developer wishing to push workloads to the Azure Container Service you can find more information at, https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-docker-cli.

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
Once the main login has been completed you will be able to login to the container registry and other Azure services.  Container registries are named in the global namespace for Azure.

If you need to create a registry then the following commands will do this for you:

```shell
$ export azure_registry_name=leafai
$ export resource_group=studioml
$ az group create --name $resource_group --location westus2
{
  "id": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/studioml",
  "location": "westus2",
  "managedBy": null,
  "name": "studioml",
  "properties": {
    "provisioningState": "Succeeded"
  },
  "tags": null
}
$ az acr create --name $azure_registry_name --resource-group $resource_group --sku Basic
 - Running ..
Create a new service principal and assign access:

```
  az ad sp create-for-rbac --scopes /subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/studioml/providers/Microsoft.ContainerRegistry/registries/leafai --role Owner --password <password>
Retrying role assignment creation: 1/36
Retrying role assignment creation: 2/36
{
  "appId": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
  "displayName": "azure-cli-2018-04-30-18-21-21",
  "name": "http://azure-cli-2018-04-30-18-21-21",
  "password": "password",
  "tenant": "tttttttt-tttt-tttt-tttt-tttttttttttt"
}
```

Use an existing service principal and assign access:
  az role assignment create --scope /subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/studioml/providers/Microsoft.ContainerRegistry/registries/leafai --role Owner --assignee <app-id>
{
  "adminUserEnabled": false,
  "creationDate": "2018-02-15T19:10:18.466001+00:00",
  "id": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/studioml/providers/Microsoft.ContainerRegistry/registries/leafai",
  "location": "westus2",
  "loginServer": "leafai.azurecr.io",
  "name": "leafai",
  "provisioningState": "Succeeded",
  "resourceGroup": "studioml",
  "sku": {
    "name": "Basic",
    "tier": "Basic"
  },
  "status": null,
  "storageAccount": null,
  "tags": {},
  "type": "Microsoft.ContainerRegistry/registries"
}
$ az acr update -n $azure_registry_name --admin-enabled true
{
  "adminUserEnabled": true,
  "creationDate": "2018-02-15T19:10:18.466001+00:00",
  "id": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/studioml/providers/Microsoft.ContainerRegistry/registries/leafai",
  "location": "westus2",
  "loginServer": "leafai.azurecr.io",
  "name": "leafai",
  "provisioningState": "Succeeded",
  "resourceGroup": "studioml",
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
$ az acr list --resource-group $resource_group --query "[].{acrLoginServer:loginServer}" --output table
AcrLoginServer
---------------------
leafai.azurecr.io
```

Pushing to Azure then becomes a process of tagging the image locally prior to the push to reflect the Azure login server, as follows:

```shell
$ docker tag leafai/studio-go-runner:0.0.33 $azure_registry_name.azurecr.io/leafai/studio-go-runner/runner:0.0.33
$ docker push $azure_registry_name.azurecr.io/leafai/studio-go-runner:0.0.33-master-1elHeQ
The push refers to a repository [leafai.azurecr.io/leafai/studio-go-runner/runner]
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

The go runner build pipeline will push images to Azure ACR when run in a shell that has logged into Azure and acr together.

Azure image repositories can be queried using the CLI tool, for example:

```shell
$ az acr repository show-tags --name $azure_registry_name --repository leafai/studio-go-runner/runner --output table
Result
--------------------
0.0.33-master-1elHeQ
```

More information about the compatibility of the registry between Azure and docker hub can be found at, https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-docker-cli.

### Kubernetes and Azure

The acs-engine tool is used to create a Kubernetes cluster when hosting on Azure.  Within Azure, acs-engine acts much like kops does for AWS.  Like kops, acs-engine will read a template, see examples/azure/kubernetes.json, and will fill in the account related information and write the resulting Azure Resource Manager templates into the '\_output' directory.  The output directory will end up containing things such as SSH keys, k8s configuration files etc.  The kubeconfig files will be generated for each region the service can be deployed to, when using the kubectl tools set your KUBECONFIG environment variable to point at the desired region.  This will happen even if the region is specified using the --location command.

When handling multiple clusters the \_output directory will end up with multiple subdirectories, one for each cluster.  The directories are auto-generated and so you will need to keep track of their names and the clusters they apply to.  After using acs-engine deploy to generate and then deploy a cluster you should identify the directory that was created in your \_output area and then use that directory name in subsequent kubectl commands, when using the KUBECONFIG environment variable.

The example examples/azure/kubernetes.json file contains an empty Azure Client ID and secret.  Before running this command you will need to create a service principal and extract client ID and sceret for it, updating this file in turn.  Those doing Azure account management and managing service principals might find the following helpful, https://github.com/Azure/acs-engine/blob/master/docs/serviceprincipal.md.

For information related to GPU workloads and k8s please review the following github page, https://github.com/Azure/acs-engine/blob/master/docs/kubernetes/gpu.md.  Using his methodology means not having to be concerned about spining up the nivida plugins and the like.

The command lines show here are using the JMESPath query language for json which you can read about here, http://jmespath.org/.

```shell
$ k8s_resource_group=test-$USER-k8s
$ acs-engine deploy --resource-group $k8s_resource_group --subscription-id $subscription_id --dns-prefix $k8s_resource_group --location eastus --auto-suffix --api-model example_k8s.json
WARN[0002] apimodel: missing masterProfile.dnsPrefix will use "test-kmutch-k8s"
INFO[0020] Starting ARM Deployment (test-kmutch-k8s-465920070). This will take some time...
INFO[0623] Finished ARM Deployment (test-kmutch-k8s-465920070). Succeeded
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

In order to access private image repositories k8s requires authenticated access to the repository.  In the following example we open access to the acr to the application created by the acs-engine.  The azurecr.io credentials can also be saved as k8s secrets as an alternative to using Azures service principals.  Using k8s secrets can be a little more error prone and opaque to the Azure platform so I tend to go with using Azure to do this.  If you do wish to go with the k8s centric approach you can find more information at, https://kubernetes.io/docs/concepts/containers/images/#using-azure-container-registry-acr.

To do the following you will need access to the service principal that operates the Client ID and secret you were given when creating the cluster.  The application ID can be found by examining the service principal in the Azure Web UI.  If you are using an account hosted by another party have them open access to the container registry you are intending to make use of.

```shell
$ az acr show --name $azure_registry_name
{
  "adminUserEnabled": true,
  "creationDate": "2018-02-12T22:13:48.208147+00:00",
  "id": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/studioml/providers/Microsoft.ContainerRegistry/registries/leafai",
  "location": "westus2",
  "loginServer": "leafai.azurecr.io",
  "name": "leafai",
  "provisioningState": "Succeeded",
  "resourceGroup": "studioml",
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
$ az role assignment create --scope /subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/studioml/providers/Microsoft.ContainerRegistry/registries/leafai --role Owner --assignee $k8s_app_id
{
  "id": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/studioml/providers/Microsoft.ContainerRegistry/registries/leafai/providers/Microsoft.Authorization/roleAssignments/0397aa24-33b4-4bd7-957b-7a51cbe39570",
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
    "scope": "/subscriptions/ssssssss-ssss-ssss-ssss-ssssssssssss/resourceGroups/studioml/providers/Microsoft.ContainerRegistry/registries/leafai"
  },
  "resourceGroup": "studioml",
  "type": "Microsoft.Authorization/roleAssignments"
}
```

The following articles can shed more light on this process and provide a more detailed walkthrough of the alternatives.

https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/
https://thorsten-hans.com/how-to-use-a-private-azure-container-registry-with-kubernetes-9b86e67b93b6

If you wish to make use of kubernetes to store Azure registry access secrets then you would use a command such as the following:

```
kubectl create secret docker-registry studioml-go-docker-key --docker-server=$azure_registry_name.azurecr.io --docker-username=[...] --docker-password=[...] --docker-email=karlmutch@gmail.com
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
