# studio-go-runner Kubernetes features

This document describes features support by the studioml go runner (runner) that are supported for generic Kubernetes installations, and builds.

## Prerequisties

This document assumes that the reader is familar with Kubernetes (k8s), docker, and Linux.

In order to perform builds and prepare docker images for remote builds inside a k8s cluster you should have the following:

An Ubuntu workstation with Docker-CE 17 or later installed, https://docs.docker.com/install/linux/docker-ce/ubuntu/
A shell account with docker accessible and appropriate rights enabled
The go compiler installed, https://github.com/golang/go/wiki/Ubuntu, snap is the preferred method, `snap install --classic go`
The python runtime installed, default on Ubuntu distributions

The next two steps will first prepare a directory from which docker images and be produced for builds, and the second will produce images that can then be tagged later on with public hosting image repositories for pulling into your build cluster.

### Build boostrapping

In order to perform a build you will need to checkout a copy of the runner using git and define several environment variables.

First a decision needs to be made as to whether you will use a fork of the open source repository and which branch is needed.  The following instructions assume that the master branch of the original open source repository is being used:

```
export GOPATH=~/project
export PATH=$GOPATH/bin:$PATH

mkdir -p ~/project/src/github.com/leaf-ai
cd ~/project/src/github.com/leaf-ai
git clone https://github.com/leaf-ai/studio-go-runner.git
cd studio-go-runner

# Get build tooling
go get github.com/karlmutch/duat
go install github.com/karlmutch/duat/cmd/semver
go install github.com/karlmutch/duat/cmd/github-release
go install github.com/karlmutch/duat/cmd/image-release
go install github.com/karlmutch/duat/cmd/stencil

# Get build dependency and package manager
go get -u github.com/golang/dep/cmd/dep


# (Optional) Get the Azure CLI tools, more information at https://github.com/Azure/azure-cli

AZ_REPO=$(lsb_release -cs)
echo "deb [arch=amd64] https://packages.microsoft.com/repos/azure-cli/ $AZ_REPO main" | \
    sudo tee /etc/apt/sources.list.d/azure-cli.list
curl -L https://packages.microsoft.com/keys/microsoft.asc | sudo apt-key add -

sudo apt-get update
sudo apt-get install apt-transport-https azure-cli


# (Optional) Get the AWS CLI Tools, more information at https://github.com/aws/aws-cli

pip install --user --upgrade awscli
```

### Building reference docker images

The next step is to produce a set of build images that can be run either locally or remotely via k8s by using Docker to create the images.

The runner supports a standalone build mode which can be used to perform local or remote builds without needing a local developer environment configured.  The Dockerfile_standalone image specification file contains the container definition to do this.

The runner also supports a developer build mode which mounts code from a developers workstation environment into a running container defined by the default Dockerfile, and Dockerfile_workstation.

```
export SEMVER=`semver`
export GIT_BRANCH=`echo '{{.duat.gitBranch}}'|stencil - | tr '_' '-' | tr '\/' '-'`

stencil -input Dockerfile | docker build -t leafai/studio-go-runner-build:$GIT_BRANCH --build-arg USER=$USER --build-arg USER_ID=`id -u $USER` --build-arg USER_GROUP_ID=`id -g $USER` -
stencil -input Dockerfile_standalone | docker build -t leafai/studio-go-runner-standalone-build:$GIT_BRANCH -
````

You will now discover that you have two docker images locally registered ready to perform full builds for you.  The first of these containers can be used for localized building during iterative development and testing.  The second image tagged with standalone-build can be used to run the build remotely without access to your local source code copy.

When build.sh is used to perform local developer builds, a container is also produced tagged as $azure_registry_name.azurecr.io/leafai/studio-go-runner/standalone-build.  This container when built will be pushed to azure and AWS docker image registries if the appropriate cloud environment tooling is available and environment variables are set, $azure\_registry\_name, and for AWS a default account configured and ECR login activated.  The image produced by the build when run will access the github source repo and will build and test the code for the branch that the developer initiating the build used.

### Image management

A script is provided within the git repo, build.sh, that does image builds and then can tag and push the images to Azure or AWS.

The script is written to make use of environment variables to push images to cloud provider image registries.  The script will check for the presense of the aws and az command line client tools before using either of these cloud providers.

The script has been used within a number of CI/CD systems and so has many commands that allow travis log folding etc.  The actual number of commands resulting in direct effects to image registries is fairly limited.

#### Azure Images

Prior to using this feature you should authenticate to the Azure infrastructure from your workstation using the 'az login' command described here, https://docs.microsoft.com/en-us/cli/azure/get-started-with-azure-cli?view=azure-cli-latest.  Your credentials will then be saved in your bootstrapping environment and used when pushing images.

The Azure image support checks that the $azure_registry_name environment variable and the az command line tool are present before being used by build.sh.

The azure_registry_name will be appended to the standard host name being used by Azure, producing a prefix for images for example $azure_registry_name.azurecr.io/leafai/studio-go-runner.

The component name will then be added to the prefix and the semantic version added to the tag as the image is pushed to Azure.

#### AWS Images

The AWS images will be pushed automatically if a default AWS account is configured.  Images will be pushed to the default region for that account, and to the registry leafai/studio-go-runner.  The semantic version will also be used within the tag for the image.

## Using k8s build and test

This section describes the k8s based builds.

In order to create a k8s cluster you will need to select a cloud provider or identify a k8s cluster running within your own infrastructure.  This document does not describe creation of a cluster, however information can be found on your cloud providers documentation web site, or on the k8s http://kubernetes.io/ documentation web site.

If you wish to test the Kubernetes features create a cluster with at least one agent node that has the nvidia plugin installed, if you are using a cloud provider use the cloud providers GPU host type when creating nodes.  Set your KUBECONFIG environment variable to point at your cluster, then create a secret to enable access from your cluster to the your AWS or Azure registry.

Your registry secrets are typically obtained from the administration portal of your cloud account.  In Azure the username and password can be found by navigating to the registry and selecting the Settings -> Access Keys section.  When using AWS the docker registry will typically be authenticated at the account level so your k8s cluster should have access automatically to the registry.

```
kubectl create secret docker-registry studioml-go-docker-key --docker-server=studio-repo.azurecr.io --docker-username=studio-repo --docker-password=long-hash-value --docker-email=karlmutch@gmail.com
```

The secret will be used by the build job to retrieve the build image and create the running container.

### k8s testing builds using the k8s job resource type

The main reasons for using a k8s cluster to build the runner is to off load longer running tests into a cluster, and secondly to obtain access to a GPU for more complete testing use cases.  When using k8s you will not be able to perform a release from within the cluster because the docker daemon is not directly accessible to you.  In these cases you would wait for the test results and do a locally controlled release using the standalone build script, build.sh.

The k8s build job can safely be run on a production cluster with GPU resources.

To bootstrap an image that can be dispatched to a k8s job the local build.sh can be used.  If the appropriate cloud environment variables are set and the build environment is successfully authenticate to the cloud the build image will be pushed to your cloud provider.

environment variables that should be set of this to work on Azure is the azure_registry_name variable.

When the local build has completed any code that needs building within the k8s cluster should be committed to the current branch.

The full build has the ability to load releases if the shell from which the build is launch has the GITHUB_TOKEN environment variables set.  You should take careful note that the k8s build will store the token as a k8s secret within the namespace of your build.  You should pay careful attention to securing your kubernetes RBAC system to prevent the token from leaking.  One option is to rotate the token on a daily basis, or use another tool to cycle your tokens automatically within the shell of the account launching these builds.

A full build can then be kicked off by using the build.yaml file to create a k8s job resource.

```
 $ stencil -input build.yaml | kubectl create -f -
```

This will then initiate the build that can be tracked using a k8s pod, for example:

```
$ kubectl describe jobs/build-studio-go-runner
Name:           build-studio-go-runner
Namespace:      default
Selector:       controller-uid=c1593355-b554-11e8-afa6-000d3a4d8ade
Labels:         controller-uid=c1593355-b554-11e8-afa6-000d3a4d8ade
                job-name=build-studio-go-runner
Annotations:    <none>
Parallelism:    1
Completions:    1
Start Time:     Mon, 10 Sep 2018 16:53:46 -0700
Pods Statuses:  0 Running / 1 Succeeded / 0 Failed
Pod Template:
  Labels:  controller-uid=c1593355-b554-11e8-afa6-000d3a4d8ade
           job-name=build-studio-go-runner
  Containers:
   build:
    Image:      quotaworkaround001.azurecr.io/leafai/studio-go-runner/standalone-build:feature-137-service-management
    Port:       <none>
    Host Port:  <none>
    Limits:
      cpu:             2
      memory:          10Gi
      nvidia.com/gpu:  1
    Environment Variables from:
      build-studio-go-runner-env  ConfigMap  Optional: false
    Environment:                  <none>
    Mounts:                       <none>
  Volumes:                        <none>
Events:
  Type    Reason            Age   From            Message
  ----    ------            ----  ----            -------
  Normal  SuccessfulCreate  25m   job-controller  Created pod: build-studio-go-runner-mpfpt

$ kubectl logs build-studio-go-runner-mpfpt -f
...
018-09-10T23:57:22+0000 INF cache_xhaust_test removed "0331071c2b0ecb52b71beafc254e0055-1" from cache \_: [host build-studio-go-runner-mpfpt]
2018-09-10T23:57:25+0000 DBG cache_xhaust_test cache gc signalled \_: [[cache_test.go:461] host build-studio-go-runner-mpfpt]
2018-09-10T23:57:25+0000 INF cache_xhaust_test bebg9jme75mc1e60rig0-11 \_: [0331071c2b0ecb52b71beafc254e0055-1 [cache_test.go:480] host build-studio-go-runner-mpfpt]
2018-09-10T23:57:26+0000 INF cache_xhaust_test TestCacheXhaust completed \_: [host build-studio-go-runner-mpfpt]
--- PASS: TestCacheXhaust (24.94s)
PASS
2018-09-10T23:57:26+0000 INF cache_xhaust_test waiting for server down to complete \_: [host build-studio-go-runner-mpfpt]
2018-09-10T23:57:26+0000 WRN cache_xhaust_test stopping k8sStateLogger \_: [host build-studio-go-runner-mpfpt] in: 
2018-09-10T23:57:26+0000 WRN cache_xhaust_test cache service stopped \_: [host build-studio-go-runner-mpfpt] in: 
2018-09-10T23:57:26+0000 WRN cache_xhaust_test http: Server closed [monitor.go:66] \_: [host build-studio-go-runner-mpfpt] in: 
2018-09-10T23:57:26+0000 INF cache_xhaust_test forcing test mode server down \_: [host build-studio-go-runner-mpfpt]
ok      github.com/leaf-ai/studio-go-runner/cmd/runner     30.064s
2018-09-10T23:57:29+0000 DBG build.go built  [build.go:138]

```

once you have seen the logs etc for the job you can delete it using the following command:

```
$ stencil -input build.yaml | kubectl delete -f -
configmap "build-studio-go-runner-env" deleted
job.batch "build-studio-go-runner" deleted
```

### k8s builds done the hard way
After creating the k8s secret to enable access to the image registry you can then run the build in an ad-hoc fashion using a command such as the following:

```
kubectl run --image=studio-repo.azurecr.io/leafai/studio-go-runner/standalone-build --attach --requests="nvidia.com/gpu=1" --limits="nvidia.com/gpu=1" build
```

Performing the build within a k8s cluster can take time due to the container creation and large images involved.  It will probably take serveral minutes, however you can check the progress by using another terminal and you will likely see something like the following:

```
$kubectl get pods
NAME                                             READY     STATUS              RESTARTS   AGE
build-67b64d446f-tfwbg                           0/1       ContainerCreating   0          2m
studioml-go-runner-deployment-847d7d5874-5lrs7   1/1       Running             0          15h
```

Once the build starts you will be able to see output like the following:

```
kubectl run --image=quotaworkaround001.azurecr.io/leafai/studio-go-runner/standalone-build --attach --requests="nvidia.com/gpu=1" --limits="nvidia.com/gpu=1" build

If you don't see a command prompt, try pressing enter.
Branch feature/137\_service_management set up to track remote branch feature/137_service_management from origin.
Switched to a new branch 'feature/137_service_management'
Warning: CUDA not supported on this platform stack="[cuda_nosupport.go:30 cuda.go:70]"
=== RUN   TestK8sConfig
--- PASS: TestK8sConfig (0.00s)
=== RUN   TestStrawMan
--- PASS: TestStrawMan (0.00s)
PASS
ok      github.com/leaf-ai/studio-go-runner/internal/runner        0.011s
```

Seeing the K8s tests complete without warning messages will let you know that they have run successfully.

The 'kubectl run' command makes use of deployment resources and so if something goes wrong you can manually manipulate the deployment using for example the 'kubectl delete deployment build' command.

## Configuration Map support

The runner uses both a global configuration map and a node specific configuration map within k8s to store state. The node specific map will superceed the global map.

The global configuration map can be found by looking for the map named 'studioml-go-runner'.  This map differs from the env maps also used by the runner in that the map once found will be watched for changes.  Currently the configuration map supports a single key, 'STATE', which is used by the runners to determine what state they should be in, or if they should terminate.

The node specific configuration can be found using the host name, ${HOSTNAME}, as a convention for naming the maps.  Care should be taken concerning this naming if the k8s deployment is modified as these names can easily be changed.

The following is an example of what can be found within the configuration map state.  In this case one of the runner pods is being specifically configured.

```
$ cat global_config.yaml
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: studioml-go-runner
data:
  STATE: Running
$ kubectl apply -f global_config.yaml
$ kubectl get -o=yaml --export cm studioml-go-runner
apiVersion: v1
data:
  STATE: Running
kind: ConfigMap
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"v1","data":{"STATE":"Running"},"kind":"ConfigMap","metadata":{"annotations":{},"name":"studioml-go-runner","namespace":"default"}}
  creationTimestamp: null
  name: studioml-go-runner
  selfLink: /api/v1/namespaces/default/configmaps/studioml-go-runner
```

Supported states include:
```
Running, DrainAndSuspend
```

Other states such as a hard abort, or a hard restart can be done using Kubernetes and are not an application state

### Security requirements

```
kubectl create clusterrolebinding default-cluster-admin --clusterrole=cluster-admin --serviceaccount=default:default
```

## Kubernetes labelling

Kubernetes supports the ability for deployments to select nodes based upon the labels of those nodes.  For example you might wish to steer work for 2 GPUs to specific machines using specific queues. To do this you can either change the deployment specification to reflect the need for multiple GPUs, or another approach is to use a label.  Labels are very useful when you wish to partition a clusters nodes temporaily to allow builds, or other specialized work to be hosted in specific places.

Using labels is a best practice as it allows your general workpool to avoid special purpose nodes by default if you use explicit labels throughout the population of nodes you have within your clusters.

An example of labelling a single GPU host and reserving for specific work can be seen below:

```
$ kubectl get nodes
NAME                                 STATUS    ROLES     AGE       VERSION
k8s-agentpool1-11296868-vmss000000   Ready     agent     3d        v1.10.8
k8s-agentpool2-11296868-vmss000000   Ready     agent     3d        v1.10.8
k8s-master-11296868-0                Ready     master    3d        v1.10.8
$ kubectl describe node k8s-agentpool2-11296868-vmss000000 |grep gpu         
 nvidia.com/gpu:     2
 nvidia.com/gpu:     2
$ kubectl describe node k8s-agentpool1-11296868-vmss000000 |grep gpu
 nvidia.com/gpu:     1
 nvidia.com/gpu:     1
$ kubectl label node k8s-agentpool1-11296868-vmss000000 leafai.affinity=production
node "k8s-agentpool1-11296868-vmss000000" labeled
```

The studioml go runner deployment can then have a label added to narrow the selection of the node on which it is deployed:

```
 template:
   metadata:
     labels:
       app: studioml-go-runner
   spec:
      imagePullSecrets:
        - name: studioml-go-docker-key
      nodeSelector:
        beta.kubernetes.io/os: linux
        leafai.affinity: production
      containers:
      - name: studioml-go-runner
...
        resources:
          requests:
            nvidia.com/gpu: 1
            memory: 8G
            cpu: 2
          limits:
            nvidia.com/gpu: 1
            memory: 16G
            cpu: 2

```

Because the deployment is being used to select the nodes on the basis of either resources or labelling the opportunity then exists for the runners assigned to them to make use of different queue names.  Again this allows with forethought for workloads to arrive on nodes that have been selected avoid your unlabelled nodes and, avoid nodes that are possibly costly or dedicated for a specific purpose.

