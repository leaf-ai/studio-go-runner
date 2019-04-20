# Continuous Integration Setup

This document describes setting up a CI pipline that can be used to prepare releases for studio go runner.

studio go runner is designed to run in resource intensive environments using GPU enabled machines and so providing a free pulically hosted pipeline for CI/CD is cost prohibitive. As an alternative, parties interested in studio go runner can make use of quay.io hosted images built automatically on github commit triggers to then trigger their own downstream build, test and deploy automation.  Downstream automation can be hosted on a self provisioned Kubernetes provisioned cluster either within the cloud or on private infrastructure.  This allows testing to be done using the CI pipeline on both local laptops, workstations and in cloud or data center environments.  The choice of quay.io as the registry for the build images is due to its support of selectively exposing only public repositories from github accounts preserving privacy.

A further option to have an entirely self hosted pipeline is also available based upon the microk8s Kubernetes distribution.  This style of pipeline is inteded to be used in circumstances where individuals with access to a single machine have limited internet bandwidth and so do not wish to host images on external services or hosts.

This document contains instructions that can be used for hardware configurations that individual users to large scale enterprises can use without incuring monthly charges from third party providers.  These instructions first detail how a quay.io, or local microk8s registry, triggered build can be setup to trigger builds on github commits.  Instructions then detail how to make use of Keel, https://keel.sh/, to pull CI images into a cluster and run the pipeline.  Finally this document describes the use of Ubers Makisu to deliver production images to the docker.io image hosting service.  docker is used as this is the most reliable of the image registries that Makisu supports, quay.io could not be made to work for this step.

# Pipeline Overview

The CI pipeline for the studio go runner project uses docker images as inputs to a series of processing steps making up the pipeline.  Following the sections describing the pipeline components there is a section describing build failure diagnosis and tracking.  This pipeline is designed for use by engineers with Kubernetes familiarity without a complex CI/CD platform and the chrome that typically accompanies domain specific platforms and languages employed by dedicated build-engineer roles.

Before using the pipeline there are several user/developer requirements for familiarity with several technologies.

1. Kubernetes

   A good technical and working knowledge is needed including knowing the Kubernetes resource abstractions as well as operational know-how
   of how to navigate between and within clusters, how to use pods and extract logs and pod descriptions to located and diagnose failures.

   Kubernetes forms a base level skill for developers and users of studio go runner open source code.

   This does not exclude users that wish to user or deploy Kubernetes free installations of studio go runner binary releases.

2. Docker and Image registry functionality

3. git and github.com

Other software systems used include

1. keel.sh
2. Mikasu from Uber
3. Go from Google

# Prerequisties

Instructions within this document make use of the go based stencil tool.  This tool can be obtained for Linux from the github release point, https://github.com/karlmutch/duat/releases/download/0.11.1/stencil-linux-amd64.

```console
$ mkdir -p ~/bin
$ wget -O ~/bin/stencil https://github.com/karlmutch/duat/releases/download/0.11.1/stencil-linux-amd64
$ chmod +x ~/bin/stencil
$ export PATH=~/bin:$PATH
```

For self hosted images using microk8s the additional git-watch tool is used to trigger CI/CD image bootstrapping as the alternative to using quay.io based image builds.

```console
$ wget -O ~/bin/stencil https://github.com/karlmutch/duat/releases/download/0.11.1/git-watch-linux-amd64
```

# A word about privacy

Many of the services that provide image hosting use Single Sign On and credentials management with your source code control platform of choice.  As a consequence of this often these services will gain access to any and all repositories private or otherwise that you might have access to within your account.  In order to preserve privacy and maintain fine grained control over the visibility of your private repositories it is recommended that when using quay.io and other services that you create a service account that has the minimal level of access to repositories as nessasary to implement your CI/CD features.

If the choice is made to use self hosted microk8s a container registry is deployed on our laptop or desktop that is not secured and relies on listening only to the local host network interface.  Using a network in-conjunction with this means you will need to secure your equipment and access to networks to prevent exposing the images produced by the build, and also to prevent other actors from placing docker images onto your machine.

# CI Image building

The studio go runner project uses Docker images to completely encapsulate builds, including a full git clone of the source comprising the release.  Using internet image registries, or alternatively the duat git-watch tool, it is possible to configure a registry to actively build an image from the git repository at that commit and to then host the resulting image.  A number of internet registries offer hosting for open source projects for free, and also offer paid hosted plans for users requiring privacy.  The second option, git-watch, serves exclusively on-premise users, and individual contributors, or small teams that do not have large financial resources to employ cloud hosted subscription sevices, or for whom the latency of moving images and data through residential internet connections is prohibitive.

Before commencing a build of the runner a reference, or base, image is created that contains all of the build tooling needed.  This image changes only when the build tooling needs upgrading or changing.  The reason for doing this is that this image is both time consuming and quite large due to dependencies on Nvidia CUDA, Python and tensorflow.  Because of this the base image build is done manually and then propogated to image registries that your build environment can access.  Typically unless there is a major upgrade most developers will be able to simply perform a docker pull from the docker.io registry to get a copy of this image. The first of instructions detail building the base image.

## CUDA and Compilation base image preparation


In order to prepare for producing product specific build images a base image is employed that contains the infrequently changing build software on which the StudioML and AI depends.

If you wish to simply use an existing build configuration then you can pull the prebuilt image into your local docker registry, or from docker hub using the following command:

```
docker pull leafai/studio-go-runner-dev-base:0.0.2
```

For situations where an on-premise or single developer machine the base image can be built with the `Dockerfile_base` file using the following command:

```console
$ docker build -t studio-go-runner-dev-base:working -f Dockerfile_base .
$ RepoImage = `docker inspect studio-go-runner-dev-base:working --format '{{ index .Config.Labels "registry.repo" }}:{{ index .Config.Labels "registry.version"}}'`
$ docker tag studio-go-runner-dev-base:working $RepoImage
$ docker rmi studio-go-runner-dev-base:working
```

If you are performing a build of a new version of the base image you can push the new version for others to use if you have the credentials needed to access the leafai account on github.

```console
$ docker tag $RepoImage docker.io/$RepoImage
$ docker login docker.io
Authenticating with existing credentials...
WARNING! Your password will be stored unencrypted in /home/kmutch/.docker/config.json.
Configure a credential helper to remove this warning. See
https://docs.docker.com/engine/reference/commandline/login/#credentials-store

Login Succeeded
$ docker push $RepoImage
The push refers to repository [docker.io/leafai/studio-go-runner-dev-base]
c7125c35d2a0: Layer already exists
1a5dc4559fc9: Layer already exists
150f158a1cca: Layer already exists
e9fe4eadf101: Layer already exists
7499c2deaea7: Layer already exists
5e0543625ca3: Layer already exists
fb88fc3593c5: Layer already exists
5f6ee5ba06b5: Layer already exists
3249250da32f: Layer already exists
31d600707965: Layer already exists
b67f23c2fd52: Layer already exists
297fd071ca2f: Layer already exists
2f0d1e8214b2: Layer already exists
7dd604ffa87f: Layer already exists
aa54c2bc1229: Layer already exists
0.0.2: digest: sha256:3f2f0f47504ebca4e6c86fd5a175001d7162049f26d657f1491578bfdfddd552 size: 3483
```

The next sections instructions, give a summary of what needs to be done in order to use the quay.io service to provision an image repository that auto-builds images from the studio go runner project, and then tests and delivers the result to the docker.io image registra.  The second section convers use cases for secured environment, along with developer workstations and laptops.

## Internet based register

The first step is to create or login to an account on quay.io.  When creating an account on quay.io it is best to ensure before starting that you have a browser window open to github.com using the account that you wish to use for accessing code on github to prevent any unintended accesses to private repositories.  As you create the account on you can choose to link it automatically to github granting application access from quay to your github authorized applications.  This is needed in order that quay can poll your projects for any pushed git commit changes in order to trigger image building.

Having logged in you can now create a repository using the label at the top right corner of your web page underneath the account related drop down menu.

The first screen will allow you to specify tgar you wish to create an image repository and assign it a name, also set the visibility to public, and to 'Link to a GitHub Repository Push', this indicates that any push of a commit or tag will result in a container build being triggered.

Pushing the next button will then cause the browser to request github to authorize access from quay to github and will prompt you to allow this authorization to be setup for future interactions between the two platform.  Again, be sure you are assuming the role of the most recently logged in github user and that the one being authorized is the one you intend to allow Quay to obtain access to.

After the authorization is enabled, the next web page is displayed which allows the organization and account to be choosen from which the image will be built.  Step through the next two screens to then select the repository that will be used and then push the continue button.

You can then specify the branch(es) that can then be used for the builds to meet you own needs.  Pushing con tinue will then allow you to select the Dockerfile that will act as your source for the new image.  When using studio go runner a Dockerfile called Dockerfile\_standalone is versioned in the source code repository that will allow a fully standalone container to be created that can be perform the entire build, test, release life cycle for the software.  usign a slash indicates the top level of the go runner repo.

Using continue will then prompt for the Context of the build which should be set to '/'.  You can now click through the rest of the selections and will end up with a fully populated trigger for the repository.

You can now trigger the first build and test cycle for the repository.  Once the repository has been built you can proceed to setting up a Kubernetes test cluster than can pull the image(s) from the repository as they are updated via git commits followed by a git push.

## Development and local image wrangling

In this use case the CI/CD based builds are performed using images pinned to pushed git commits that have been built within a Kubernetes cluster.  In order to support local Kubernetes clusters the microk8s tool is used, https://microk8s.io/.

Uses cases for local clusters include secured environments, snap based installation of the microk8s tool can be done by downloading the snap file.  Another option is to download a git export of the microk8s tool and build it within your secured environment.  If you are using a secured environment adequate preparations should also be made for obtaining copies of any images that you will need for running your applications and also reference images needed by the microk8s install such as the images for the DNS server, the container registry, the Makisu image from docker hub and other images that will be used.  In order to be able to do this you can pre-pull images for build and push then to a private registry. If you need access to multiple registries, you can create one secret for each registry. Kubelet will merge any imagePullSecrets into a single virtual .docker/config.json. For more information please see, https://kubernetes.io/docs/concepts/containers/images/#using-a-private-registry.

While you can run within a walled garden secured network environment the microk8s cluster does use an unsecured registry which means that the machine and any accounts on which builds are running should be secured independently.  If you wish to secure images that are produced by your pipeline then you should modify your ci\_containerize\_microk8s.yaml, or a copy of the same, file to point at a private secured registry, such as a self hosted https://trow.io/ instance.

The CI bootstrap step is the name given to the initial CI pipeline image creation step. In order to ensure that your local environment is configured to communicate with the kubernetes cluster the following commands should be run to setup your Kubernetes context.

```console
export LOGXI='*=DBG'
export LOGXI_FORMAT='happy,maxcol=1024'

export SNAP=/snap
export PATH=$SNAP/bin:$PATH

export KUBE_CONFIG=~/.kube/microk8s.config
export KUBECONFIG=~/.kube/microk8s.config

microk8s.config > $KUBECONFIG
microk8s.enable registry storage dns gpu
```

The first step is the loading of the base image containing the needed build tooling.  The base image can be loaded into your local docker environment and then subsequently pushed to the cluster registry.

```console
$ docker build -t studio-go-runner-dev-base:working -f Dockerfile_base .
$ RegistryPrefix=`docker inspect studio-go-runner-dev-base:working --format '{{ index .Config.Labels "registry.repo" }}:{{ index .Config.Labels "registry.version"}}'`
$ docker tag studio-go-runner-dev-base:working $RegistryPrefix
$ docker tag studio-go-runner-dev-base:working localhost:32000/$RegistryPrefix
$ docker rmi studio-go-runner-dev-base:working
$ docker push localhost:32000/$RegistryPrefix
```

Once our base image is loaded and has been pushed into the kubernetes container registry git-watch can be used to initiate image builds inside the cluster that, use the base image, git clone source code from fresh commits, and build scripts etc to create an entirely encapsulated CI image.

The git-watch tool monitors a git repository and polls looking for pushed commits.  When the code is cloned to be built a Makisu pod is started for creating images within the Kubernetes cluster.  The Makisu build then pushes build images to a user nominated repository which becomes the triggering point for the CI/CD downstream steps.

Because localized images are intended to assist in conditions where image transfers are expensive time wise it is recommended that the first step be to deploy the redis cache as a Kubernetes service.  This cache will be employed by Makisu when the ci\_containerize\_microk8s.yaml file is used as a task template.  The cache pods can be started by using the following commands:

```console
$ microk8s.kubectl apply -f ci_containerize_cache.yaml
namespace/makisu-cache created
pod/redis created
service/redis created
```

Configuring the watcher occurs by modification of the ci\_containerize\_log.yaml file and also specifying the git repository location to be polled as well as the branch name of interest denoted by the '^' character.  The yaml file contains references to the location of the container registry that will recieve the image only it has been built.  The intent is that a downstream Kubernetes based solution such as keel.sh will further process the image as part of a CI/CD pipeline, please see the section describing Continuous Integration.

```console
$ export Registry=`cat registry_local.yaml`
$ git-watch -v --job-template ci_containerize_microk8s.yaml https://github.com/leaf-ai/studio-go-runner.git^master
```

# Continuous Integration

The presence of a quay.io, or locally hosted microk8s image repository will allow a suitably configured Kubernetes cluster to query for bootstrapped build images and to use these for building, testing, and integration.

The studio go runner standalone build image can be used within a go runner deployment to perform testing and validation against a live minio (s3 server) and a RabbitMQ (queue server) instances deployed within a single Kubernetes namespace.  The definition of the deployment is stored within the source code repository, in the file ci\_keel.yaml, or its equivalent for locally deployed pipelines ci\_keel\_microk8s.yaml.

The build deployment contains an annotated kubernetes deployment of the build image that when deployed concurrently with keel can react to fresh build images to cycle automatically through build, test, release image cycles.

The commands that you might performed to this point in order to deploy keel into an existing Kubernetes deploy might well appear as follows:

```
mkdir -p ~/project/src/github.com/keel-hq
cd ~/project/src/github.com/keel-hq
git clone https://github.com/keel-hq/keel.git
cd keel/deployment
kubectl create -f deployment-rbac.yaml
mkdir -p ~/project/src/github.com/leaf-ai
cd ~/project/src/github.com/leaf-ai
git clone https://github.com/leaf-ai/studio-go-runner.git
cd studio-go-runner
git checkout [branch name]
# Follow the instructions for setting up the Prerequisites for compilation in the main README.md file
```

Keel is documented at https://keel.sh/, installation instruction can also be found there, https://keel.sh/guide/installation.html.  Once deployed keel can be left to run as a background service observing Kubernetes deployments that contain annotations it is designed to react to.  Keel will watch for changes to image repositories that for and will automatically upgrade the Deployment pods as new images are seen causing the CI/CD build inside the pod to be triggered.

The image name for the build Deployment in the ci\_keel.yaml file is used by keel to monitor updates to images found in the configured Registry. The keel labels within the ci\_keel.yaml file dictate under what circumstances the keel server will trigger a new pod for the build and test to be created in response to the reference build image changing as git commit and push operations are performed.  Information about these labels can be found at, https://keel.sh/v1/guide/documentation.html#Policies.

The next step would be to edit the ci\_keel.yaml, or the ci\_keel\_microk8s.yaml file to reflect the branch name on which the development is being performed or the release prepared, and then deploy the integration stack.  

The Registry value, $Registry, is used to pass your docker hub username, and password to keel orchestrated containers and the release image builder, Makisu, using a kubernetes secret.  An example of how to set this value is included in the next section, continue on for more details.  Currently only dockerhub, and microk8s registries are supported for pushing release images to.

When a build finishes the stack will scale down the testing dependencies it uses for queuing and storage and will keep the build container alive so that logs can be examined.  The build activities will disable container upgrades while the build is running and will then open for upgrades once the build steps have completed to prevent premature termination.  When the build, and test has completed and pushed commits have been seen for the code base then the pod will be shutdown for the latest build and a new pod created.

If the envronment variable GITHUB\_TOKEN is present when deploying an integration stack it will be placed as a Kubernetes secret into the integration stack.  If the secret is present then upon successful build and test cycles the running container will attempt to create and deploy a release using the github release pages.

When the build completes the pods that are present that are only useful during the actual build and test steps will be scaled back to 0 instances.  The CI script, ci.sh, will spin up and down specific kubernetes jobs and deployments when they are needed automatically by using the Kubernetes kubectl command.  Bceuase of this your development and build cluster will need access to the Kubernetes API server to complete these tasks.  The Kubernetes API access is enabled by the ci\_keel.yaml file when the standalone build container is initialized.

Before using the registry setting you should copy registry-template.yaml to registry.yaml, and modify the contents.

If the environment is shared between multiple people the namespace can be assigned using the petname tool, github.com/karlmutch/petname, as shown below.

```
cat registry.yaml
index.docker.io:
  .*:
    security:
      tls:
        client:
          disabled: false
      basic:
        username: docker_account_name
        password: docker_account_password
export Registry=`cat registry.yaml`
export GITHUB_TOKEN=a6e5f445f68e34bfcccc49d01c282ca69a96410e
export K8S_NAMESPACE=ci-go-runner-`petname`
stencil -input ci_keel.yaml -values Registry=${Registry},Namespace=$K8S_NAMESPACE | kubectl apply -f -

export K8S_POD_NAME=`kubectl --namespace=$K8S_NAMESPACE get pods -o json | jq '.items[].metadata.name | select ( startswith("build-"))' --raw-output`
kubectl --namespace $K8S_NAMESPACE logs -f $K8S_POD_NAME
```

or, if you do not wish to use a registry for pushing the tested image

```
export Registry=`cat registry.yaml`
stencil -input ci_keel.yaml -values Namespace=ci-go-runner-`petname`| kubectl apply -f -
export K8S_NAMESPACE=`kubectl get ns -o json | jq --raw-output '.items[] | select(.metadata.name | startswith("ci-go-runner-")) | .metadata.name'`

export K8S_POD_NAME=`kubectl --namespace=$K8S_NAMESPACE get pods -o json | jq '.items[].metadata.name | select ( startswith("build-"))' --raw-output`
kubectl --namespace $K8S_NAMESPACE logs -f $K8S_POD_NAME
```

## Locally deployed keel testing and CI

These instructions will be useful to those using a locally deployed Kubernetes distribution such as microk8s.  If you wish to use microk8s you should first deploy using the workstations instructions found in this souyrce code repository at docs/workstation.md.  You can then return to this section for further information on deploying the keel based CI/CD within your microk8s environment.

In the case that a test of a locally pushed docker image is needed you can build your image locally and then when the build.sh is run it will do a docker push to a microk8s cluster instance running on your workstation or laptop.  In order for the keel deployment to select the locally hosted image registry you set the Image variable for stencil to substitute into the ci\_keel\_microk8s.yaml file.

When the release features are used the CI/CD system will make use of the Makisu image builder, authored by Uber.  Makisu allows docker containers to build images entirely within an existing container with no specialized dependencies and also without needing dind (docker in docker), or access to a docker server socket.

```console
$ ./build.sh
$ export GITHUB_TOKEN=a6e5f445f68e34bfcccc49d01c282ca69a96410e
$ export Registry=`cat registry_local.yaml`
$ stencil -input ci_keel_microk8s.yaml -values Registry=${Registry},Image=localhost:32000/leafai/studio-go-runner-standalone-build:${GIT_BRANCH},Namespace=ci-go-runner-`petname`| kubectl apply -f -
```

If you are using the Image bootstrapping features of git-watch the commands would appear as follows:

```console
$ export GITHUB_TOKEN=a6e5f445f68e34bfcccc49d01c282ca69a96410e
$ export Registry=`cat registry_local.yaml`
$ stencil -input ci_keel_microk8s.yaml -values Registry=$Registry,Namespace=ci-go-runner | kubectl apply -f -
```

In the above case the branch you are currently on dictates which bootstrapped images based on their image tag will be collected and used for CI/CD operations.
