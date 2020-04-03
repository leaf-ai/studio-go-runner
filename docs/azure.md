# Azure support for studio-go-runner

This document describes the Azure specific steps for the installation and use of the studio-go-runner within Azure.

Before using these instruction you should have an Azure account and have full access to its service principal.  These instruction will guide you through the creation of a Kubernetes cluster using Microsoft specific tools.  After completing them you will be able to use the kubectl and other generic tools for installation of the go runner.

This Go runner, and the Python runner found within the reference implementation of StudioML, have been tested on the Microsoft Azure cloud.

After completing the instructions in this document you may return to the main README.md file for further instructions.

<!--ts-->

Table of Contents
=================

* [Azure support for studio-go-runner](#azure-support-for-studio-go-runner)
* [Table of Contents](#table-of-contents)
  * [Prerequisites](#prerequisites)
  * [Planning](#planning)
  * [Installation Prerequisites](#installation-prerequisites)
    * [Automatted installation](#automatted-installation)
    * ['The hard way' Installation](#the-hard-way-installation)
      * [RabbitMQ Deployment](#rabbitmq-deployment)
      * [Minio Deployment](#minio-deployment)
  * [Compute cluster deployment](#compute-cluster-deployment)
    * [Kubernetes and Azure](#kubernetes-and-azure)
    * [Azure Kubernetes Private Image Registry deployments](#azure-kubernetes-private-image-registry-deployments)
* [Manifest and suggested deployment artifacts](#manifest-and-suggested-deployment-artifacts)
  * [RabbitMQ Server](#rabbitmq-server)
  * [Minio S3 Server](#minio-s3-server)
  * [Workers](#workers)
  * [Security Note](#security-note)
  * [Software Manifest](#software-manifest)
  * [CentOS and RHEL 7.0](#centos-and-rhel-70)
<!--te-->
## Prerequisites

The Azure installation process will generate a number of keys and other valuable data during the creation of cloud based compute resources that will need to be sequestered in some manner.  In order to do this a long-lived host should be provisioned provisioned for use with the administration steps detailed within this document.
Your linux account should have an ssh key generated, see ssh-keygen man pages.

Azure can run Kubernetes as a platform for fleet management of machines and container orchestration using AKS supporting regions with machine types that have GPU resources. kubectl can be installed using instructions found at:

- kubectl https://kubernetes.io/docs/tasks/tools/install-kubectl/

Docker is also used to manage images from an administration machine. For Ubuntu the instructions can be found at the following location.

- Docker Ubuntu Installation, https://docs.docker.com/install/linux/docker-ce/ubuntu/#install-docker-engine---community

If the decision is made to use CentOS 7 then special accomodation needs to be made. These changes are described at the end of this document.  In addition, the automatted scripts within the cloud directory are designed to deploy Ubuntu Azure master images.  These will need modification when using CentOS.

Instructions on getting started with the azure tooling, at least Azure CLI 2.0.73, needed for operating your resources can be found as follows:

- AZ CLI https://github.com/Azure/azure-cli#installation

If you are a developer wishing to push workloads to the Azure Container Service you can find more information at, https://docs.microsoft.com/en-us/azure/container-registry/container-registry-get-started-docker-cli.

The Kubernetes eco-system has a customization tool known as kustomize that is used to adapt clusters to the exact requirements of customers.  This tool can be installed using the following commands:

```shell
wget -O /usr/local/bin/kustomize https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv3.5.4/kustomize_kustomize.v3.5.4_linux_amd64
chmod +x /usr/local/bin/kustomize
export PATH=$PATH:/usr/local/bin
```

For the purposes of exchanging files with the S3 Minio server the minio client is available and can be installed using the following commands:

```shell
wget -O /usr/local/bin/mc https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x /usr/local/bin/mc
```

Now that the tooling is installed there are three major components for which installation occurs, a rabbitMQ server, a Minio S3 file server, and the compute cluster.  The following sections detail these in order.

It is also worth noting that the requirements for the node pool network subnet can be have on IP addresses that are assigned, a subnet of sufficient size should be allocated for use by the node pools being used..  Each node within the node pool will be assigned a mnimum of 20 IPs unless efforts are made to restrict the creation of the node pool to bing done using the Azure command line tool.

## Planning

The Azure Kubernetes Service (AKS) has specific requirements in relation to networking that are critical to observe, this cannot be emphasized strongly enough.  For information about the use of Azure CNI Networking please review, https://docs.microsoft.com/en-us/azure/aks/configure-azure-cni.  Information about the use of bastion hosts to protect the cluster please see, https://docs.microsoft.com/en-us/azure/aks/operator-best-practices-network.  For information about the network ports that need to be opened, please review, https://docs.microsoft.com/en-us/azure/aks/limit-egress-traffic.

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

### Automatted installation

Installation of the RabbitMQ (rmq) queue server, and the minio S3 server, both being components within a StudioML deployment using runners, is included when using scripts found in this repositories cloud sub directory.  If you wish to perform a ground up installation without checking out the studio-go-runner repository you can directly download the rmq and minio installation and run it using the following commands:

```shell
# The following command will create a temporary directory to run the install from and will move to it
cd `mktemp -d`
wget -O install_custom.sh https://raw.githubusercontent.com/leaf-ai/studio-go-runner/master/cloud/install.sh
wget -O README.md https://raw.githubusercontent.com/leaf-ai/studio-go-runner/master/cloud/README.md
```

You should now edit the installation file that was downloaded and follow the instructions included within it.  After changes are written to disk you can now return to running the installation.

```shell
chmod +x ./install_custom.sh
./install_custom.sh
# Print the directory used to perform the installation
pwd
# Return to the users directory
cd -
```

More information can be found at, https://github.com/leaf-ai/studio-go-runner/blob/master/cloud/README.md.

### 'The hard way' Installation

Once the subscription ID is selected the next step is to generate for ourselves an identifier for use with Azure resource groups etc that identifies the current userand local host to prevent collisions.  This can be done using rthe following commands:

```shell
uniq_id=`md5sum <(echo $subscription_id $(ip maddress show eth0)) |  cut -f1 -d\  | cut -c1-8`
````

#### RabbitMQ Deployment

Azure has a prepackaged version of the Bitnami distribution of RabbitMQ available.  

Before using the marketplace version you will need to retrieve your SSH public key and have it accessible when prompted.

```shell
cat $HOME/.ssh/id_rsa.pub
```

To begin the launch of this service use the Azure search bar to locate the Marketplace image, enter "RabbitMQ Certified by Bitnami" and click on the search result for marketplace.

Click on 'create' to move to the first configuration screen. Fill in the Resource group, and a Virtual Machine Name of your choice.  Next select the Region to be (US) East US.  It is also advised to change the machine type to be an A2\_v2.

At the bottom of this screen there are administration account details that should be filled in.  Use a username of your choice and paste into the SSH Public Key field you public SSH key, shown above.

Begin moving through the configuration screens stopping in the management screen to turn off 'Auto-Shutdown' and then continue and finally use the Create button on the last screen to initialize the machine.

Once the deployment has completed a public IP address will be assigned by Azure and can be seen by going into the vnet interface attached to the machine and looking at the IP Configurations section.  This can be found by clicking on the device listed inside the connected device pane of the vnet overview panel. Once you can see the public IP address of the screen take a note of that and then on the Configuration menu list item on the left side of the xx-ip configuration web page panel.

The ip configuration screen on Azure should now be used to set the public IP address assignment to Static in order that the machine is consistently available at the IP address it initially used.  Press the save button which is displayed at the top left of the configuration panel.

Access to the web administration interface for this machine and also to the queue API interface should now be enabled in the network security group for the machine.  To get to this screen return to the Azure web UI resource group you created and select the resource group to reveal the list of assets, in here you will see a network security group. Click on it and then the details screen will be shown.  Choose the inbound security rules menu item on the left hand side of the details view and you will see an add option for each port that will be exposed.  The add option will allow ports to be added, as you add the ports the only detail that usually needs changing is the port number in the 'Destination Port Ranges', and possibly the name of the rule to make things clear as to which port is being opened.  Once these are entered press the Add button at the bottom of the panel.

You should open ports 15672, and 5672.  The second port will require a priority to be set, add 1 to the default priority value inserted.

Three variables are required from the RabbitMQ install that will be used later, the IP Address of the server, and the user name, password pair.  Commands later on within this document will refer to these values so you might want to record them as environment variables.
Access to the machine from the administration workstation can now be gained by using the ssh command bundled with your Ubuntu management workstation, for example:

```shell
ssh 40.117.178.107
The authenticity of host '40.117.178.107 (40.117.178.107)' can't be established.
ECDSA key fingerprint is SHA256:A9u3R6/pjKW37mvMrIq5ZJarx4TmHSmdUVTAuTPt9HY.
Are you sure you want to continue connecting (yes/no)? yes
Warning: Permanently added '40.117.178.107' (ECDSA) to the list of known hosts.
Welcome to Ubuntu 16.04.6 LTS (GNU/Linux 4.15.0-1060-azure x86_64)

The programs included with the Ubuntu system are free software;
the exact distribution terms for each program are described in the
individual files in /usr/share/doc/*/copyright.

Ubuntu comes with ABSOLUTELY NO WARRANTY, to the extent permitted by
applicable law.

       ___ _ _                   _
      | _ |_) |_ _ _  __ _ _ __ (_)
      | _ \ |  _| ' \/ _` | '  \| |
      |___/_|\__|_|_|\__,_|_|_|_|_|

  *** Welcome to the Bitnami RabbitMQ 3.8.0-0 ***
  *** Service accessible using hostname 40.117.178.107 , check out https://docs.bitnami.com/azure/infrastructure/rabbitmq/administration/connect-remotely/ ***
  *** Documentation:  https://docs.bitnami.com/azure/infrastructure/rabbitmq/ ***
  ***                 https://docs.bitnami.com/azure/ ***
  *** Bitnami Forums: https://community.bitnami.com/ ***
To run a command as administrator (user "root"), use "sudo <command>".
See "man sudo_root" for details.

bitnami@rabbitMQ:~$
```

Instructions for obtaining the administration User ID can be found at https://docs.bitnami.com/azure/faq/get-started/find-credentials/.

```shell
export rabbit_host=40.117.178.107
export rabbit_user=user
export rabbit_password=password
```

You can now test access to the server by going to a browser and use the url, http://[the value of $rabbit_host]:15672.  This will display a logon screen that you can enter the user name and the password into, thereby testing the access to the system.

#### Minio Deployment

To begin the launch of this service use the Azure search bar to locate the Marketplace image, enter "Ubuntu Server 18.04 LTS" and click on the search result for marketplace.  Be sure that the one choosen is provided by Canonical and no other party.  You will be able to identify the exact version by clicking on the "all results" option in the search results drop down panel.  When using this option a list of all the matching images will be displayed with the vendor name underneath the icon.

Click on 'create' to move to the first configuration screen. Fill in the Resource group, and a Virtual Machine Name of your choice.  Next select the Region to be (US) East US. The default machine type of D2s_v3 is appropriate until your requirements are fully known.

At the bottom of this screen there are administration account details that should be filled in.  Use a username of your choice and paste into the SSH Public Key field you public SSH key, shown above.

Clicking next will take you to the Disks screen.  You will need to use the Disks configuration screen to add an empty disk, "create and attach a disk", with 1TB of storage or more to hold any experiment data that is being generated.  When prompted for the details of the disk use the "Storage Type" drop down to select an empty disk, "None"i, and change the size using the menus underneath that option.

Next move to the Networking screen and choose the "Public inbound ports" option to allow SSH to be exposed in order that you can SSH into this machine.

Continue moving through the configuration screens stopping in the management screen to turn off 'Auto-Shutdown' and then continue and finally use the Create button on the last screen to initialize the machine.

Once the deployment has completed a public IP address will be assigned by Azure and can be seen by going into the vnet interface attached to the machine and looking at the IP Configurations section.  This can be found by clicking on the device listed inside the connected device pane of the vnet overview panel. Once you can see the public IP address of the screen take a note of that and then on the Configuration menu list item on the left side of the xx-ip configuration web page panel.

The ip configuration screen on Azure should now be used to set the public IP address assignment to Static in order that the machine is consistently available at the IP address it initially used.  Press the save button which is displayed at the top left of the configuration panel.

Access to the web administration interface for this machine and also to the queue API interface should now be enabled in the network security group for the machine.  To get to this screen return to the Azure web UI resource group you created and select the resource group to reveal the list of assets, in here you will see a network security group. Click on it and then the details screen will be shown.  Choose the inbound security rules menu item on the left hand side of the details view and you will see an add option for each port that will be exposed.  The add option will allow ports to be added, as you add the ports the only detail that usually needs changing is the port number in the 'Destination Port Ranges', and possibly the name of the rule to make things clear as to which port is being opened.

Following the above instruction you should now make the minio server port available for use through the network security group associated with the network interface, opening port 9000.

Access to the machine from the administration workstation can now be done, for example:

```shell
ssh 40.117.155.103
The authenticity of host '40.117.155.103 (40.117.155.103)' can't be established.
ECDSA key fingerprint is SHA256:j6XftRWhoyoLmlQtkfvtL5Mol0l2rQ3yAl0+QDo6EV4.
Are you sure you want to continue connecting (yes/no)? yes
Warning: Permanently added '40.117.155.103' (ECDSA) to the list of known hosts.
Welcome to Ubuntu 18.04.3 LTS (GNU/Linux 5.0.0-1018-azure x86_64)

 * Documentation:  https://help.ubuntu.com
 * Management:     https://landscape.canonical.com
 * Support:        https://ubuntu.com/advantage

  System information as of Thu Oct 17 00:26:33 UTC 2019

  System load:  0.07              Processes:           128
  Usage of /:   4.2% of 28.90GB   Users logged in:     0
  Memory usage: 4%                IP address for eth0: 10.0.0.4
  Swap usage:   0%

7 packages can be updated.
7 updates are security updates.



The programs included with the Ubuntu system are free software;
the exact distribution terms for each program are described in the
individual files in /usr/share/doc/*/copyright.

Ubuntu comes with ABSOLUTELY NO WARRANTY, to the extent permitted by
applicable law.

To run a command as administrator (user "root"), use "sudo <command>".
See "man sudo_root" for details.

kmutch@MinioServer:~$
```

The following commands should now be run to upgrade the OS to the latest patch levels:

```shell
sudo apt-get update
sudo apt-get upgrade
sudo useradd --system minio-user --shell /sbin/nologin
```

We now add the secondary 1TB storage allocated during machine creation using the fdisk command and then have the partition mounted automatically upon boot. The fdisk utility is menu driven so this is shown as an example.  Most fields can be defaulted.

```shell
kmutch@MinioServer:~$ sudo fdisk /dev/sdc

Welcome to fdisk (util-linux 2.31.1).
Changes will remain in memory only, until you decide to write them.
Be careful before using the write command.

Device does not contain a recognized partition table.
Created a new DOS disklabel with disk identifier 0xab23eb4b.

Command (m for help): n
Partition type
   p   primary (0 primary, 0 extended, 4 free)
   e   extended (container for logical partitions)
Select (default p): p
Partition number (1-4, default 1): 
First sector (2048-2145386495, default 2048): 
Last sector, +sectors or +size{K,M,G,T,P} (2048-2145386495, default 2145386495): 

Created a new partition 1 of type 'Linux' and of size 1023 GiB.

Command (m for help): w
The partition table has been altered.
Calling ioctl() to re-read partition table.
Syncing disks.
kmutch@MinioServer:~$ sudo mkfs.ext4 /dev/sdc1
mke2fs 1.44.1 (24-Mar-2018)
Discarding device blocks: done                            
Creating filesystem with 268173056 4k blocks and 67043328 inodes
Filesystem UUID: e1af35dc-344b-45d6-aec6-8c39b1ad30d6
Superblock backups stored on blocks: 
        32768, 98304, 163840, 229376, 294912, 819200, 884736, 1605632, 2654208, 
        4096000, 7962624, 11239424, 20480000, 23887872, 71663616, 78675968, 
        102400000, 214990848

Allocating group tables: done                            
Writing inode tables: done                            
Creating journal (262144 blocks): done
Writing superblocks and filesystem accounting information: done     

kmutch@MinioServer:~$ sudo su
# mkdir /data
# id=`blkid /dev/sdc1 | cut -f2 -d\"`
# cat << EOF >> /etc/fstab
UUID=$id /data    auto nosuid,nodev,nofail,x-gvfs-show 0 0
EOF
root@MinioServer:/home/kmutch# mount -a
```

The minio installation can now begin

```shell
sudo su
useradd --system minio-user --shell /sbin/nologin
wget -O /usr/local/bin/minio https://dl.minio.io/server/minio/release/linux-amd64/minio
chmod +x /usr/local/bin/minio
chown minio-user:minio-user /usr/local/bin/minio
mkdir /data/minio
mkdir /etc/minio
chown minio-user:minio-user /data/minio
chown minio-user:minio-user /etc/minio
cat << EOF >> /etc/default/minio
MINIO_VOLUMES="/data/minio/"
MINIO_OPTS="-C /etc/minio"
MINIO_ACCESS_KEY=229A0YHNJZ1DEXB80WFG
MINIO_SECRET_KEY=hsdiPjaZjd8DKD04HwW8GF0ZA9wPv8FCgYR88uqR
EOF
wget -O /etc/systemd/system/minio.service https://raw.githubusercontent.com/minio/minio-service/master/linux-systemd/minio.service
systemctl daemon-reload
systemctl enable minio
sudo service minio start
```

Once the minio server has been initiated information related to a generated access key and secret key will be generated for this installation.  These values should be extracted and used to access the file server:

```shell
sudo cat /data/minio/.minio.sys/config/config.json| grep Key
                "accessKey": "229A0YHNJZ1DEXB80WFG",
                "secretKey": "hsdiPjaZjd8DKD04HwW8GF0ZA9wPv8FCgYR88uqR",
                                "routingKey": "",
```

These values should be recorded and kept in a safe location on the administration host for use by StudioML clients and experimenters.  You also have the option of changing the values in this file to meet your own requirements and then restart the server.  These values will be injected into your experiment host hocon configuration file.

```shell
export minio_access_key=229A0YHNJZ1DEXB80WFG
export minio_secret_key=hsdiPjaZjd8DKD04HwW8GF0ZA9wPv8FCgYR88uqR
```

If you wish to make use of the mc, minio client, to interact with the server you can add the minio host details to the mc configuration file to make access easier, please refer to the minio mc guide found at, https://docs.min.io/docs/minio-client-quickstart-guide.html.

```shell
mc config host add studio-s3 http://40.117.155.103:9000 ${minio_access_key} ${minio_secret_key}
mc mb studio-s3/mybucket
mc ls studio-s3
mc rm studio-s3/mybucket
```

Should you wish to examine the debug logging for your minio host the following command can be used:

```shell
sudo service minio status
```

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
docker pull leafai/azure-studio-go-runner:0.9.21
docker tag leafai/azure-studio-go-runner:0.9.21 $azure_registry_name.azurecr.io/${azure_registry_name}/studio-go-runner:0.9.21
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
```

```shell
cat << EOF > examples/azure/map.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: studioml-env
data:
  AMQP_URL: "amqp://${rabbit_user}:${rabbit_password}@${rabbit_host}:5672/"
EOF
cat << EOF > examples/azure/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- deployment-1.13.yaml
patchesStrategicMerge:
- map.yaml
images:
- name: studioml/studio-go-runner
  newName:  ${azure_registry_name}.azurecr.io/${azure_registry_name}/studio-go-runner:0.9.21
EOF
kubectl apply -f <(kustomize build examples/azure)
kubectl get pods
```

### Azure Kubernetes Private Image Registry deployments

In order to access private image repositories k8s requires authenticated access to the repository.  In the following example we open access to the acr to the application created by the aks-engine.  The azurecr.io credentials can also be saved as k8s secrets as an alternative to using Azure service principals.  Using k8s secrets can be a little more error prone and opaque to the Azure platform so I tend to go with using Azure to do this.  If you do wish to go with the k8s centric approach you can find more information at, https://kubernetes.io/docs/concepts/containers/images/#using-azure-container-registry-acr.

The following article shows how the Azure AKS cluster can be attached to the Azure Container Registry from which images are being served.

https://thorsten-hans.com/aks-and-acr-integration-revisited

az aks update --resource-group $k8s_resource_group --name $aks_cluster_group --attach-acr $registryId

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

CentOS and RHEL 7.0
-------------------

Prior to running the Docker installation the containerd runtime requires the cgroups seline library and profiles to be installed using a archived repository for packages as follows:

```shell
yum install http://http://vault.centos.org/centos/7.6.1810/extras/x86_64/Packages/container-selinux-2.107-1.el7_6.noarch.rpm
````

Should you be using an alternative version of CentOS this server contains packages for many variants and versions of CentOS and can be browsed.

Copyright &copy 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
