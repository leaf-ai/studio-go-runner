# Continuous Integration Setup

This document describes setting up a CI pipline that can be used to prepare releases for studio go runner.

studio go runner is designed to run in resource intensive environments using GPU enabled machines and so providing a free hosted pipeline for CI/CD is cost prohibitive. As an alternative, parties interested in studio go runner can make use of quay.io hosted images built automatically on github commit triggers to then trigger their own downstream build, test and deploy automation.  Downstream automation can be hosted on a self provisioned Kubernetes provisioned cluster either within the cloud or on private infrastructure.  This allows testing to be done using the CI pipeline on both local laptops, workstations and in cloud or data center environments.  The choice of quay.io as the registry for the build images is due to its support of selectively exposing only public repositories from github accounts preserving privacy.

This document contains instructions that can be used for hardware configurations that individual users to large scale enterprises can use without incuring monthly charges from third party providers.  These instructions first detail how a quay.io trigger can be setup to trigger builds on github commits.  Instructions then detail how to make use of Keel, https://keel.sh/, to pull CI images into a cluster and run the pipeline.  Finally this document describes the us of Ubers Makisu to deliver production images to the docker.io image hosting service.  docker is used as this is the most reliable of the image registries that Makisu supports, quay.io could not be made to work for this step.


Instructions within this document make use of the go based stencil tool.  This tool can be obtained for Linux from the github release point, https://github.com/karlmutch/duat/releases/download/0.11.1/stencil-linux-amd64.

```
mkdir -p ~/bin
wget -O ~/bin/stencil https://github.com/karlmutch/duat/releases/download/0.11.1/stencil-linux-amd64
chmod +x ~/bin/stencil
export PATH=~/bin:$PATH
```

# A word about privacy

Many of the services that provide image hosting use Single Sign On and credentials management with your source code control platform of choice.  As a consequence of this often these services will gain access to any and all repositories private or otherwise that you might have access to within your account.  In order to preserve privacy and maintain fine grained control over the visibility of your private repositories it is recommended that when using quay.io and other services that you create a service account that has the minimal level of access to repositories as nessasary to implement your CI/CD features.

# CI Image building

The studio go runner project uses Docker images to encapsulate builds within an immutable archive format.  Using internet image registries, or alternatively the duat git-watch tool it is possible to configure a registry to actively build an image from the git repository at that commit and to then host the resulting image.  A number of internet registries offer hosting for open source projects for free, and also offer paid hosted plans for users requiring privacy.  The second option allows individual contributors or small teams that do not have large financial resources to employ subscription sevices, or for whom the latency of moving images and data through residential intyernet connections is prohibitive.

The first set of instructions, give a summary of what needs to be done in order to use the quay.io service to provision an image repository that auto-builds images from the studio go runner project, and then tests and delivers the result to the docker.io image registra.  The second section convers use cases for developer workstations and laptops.

## Internet based register

The first step is to create or login to an account on quay.io.  When creating an account on quay.io it is best to ensure before starting that you have a browser window open to giuthub.com using the account that you wish to use for accessing code on github to prevent any unintended accesses to private repositories.  As you create the account on you can choose to link it automatically to github granting application access from quay to your github authorized applications.  This is needed in order that quay can poll your projects for any changes in order to trigger image building.

Having logged in you can now create a repository using the label at the top right corner of your web page underneath the account related drop down menu.

The first screen will allow you to specify tgar you wish to create an image repository and assign it a name, also set the visibility to public, and to 'Link to a GitHub Repository Push', this indicates that any push of a commit or tag will result in a container build being triggered.

Pushing the next button will then cause the browser to request github to authorize access from quay to github and will prompt you to allow this authorization to be setup for future interactions between the two platform.  Again, be sure you are assuming the role of the most recently logged in github user and that the one being authorized is the one you intend to allow Quay to obtain access to.

After the authorization is enabled, the next web page is displayed which allows the organization and account to be choosen from which the image will be built.  Step through the next two screens to then select the repository that will be used and then push the continue button.

You can then specify the branch(es) that can then be used for the builds to meet you own needs.  Pushing con tinue will then allow you to select the Dockerfile that will act as your source for the new image.  When using studio go runner a Dockerfile called Dockerfile\_standalone is versioned in the source code repository that will allow a fully standalone container to be created that can be perform the entire build, test, release life cycle for the software.  usign a slash indicates the top level of the go runner repo.

Using continue will then prompt for the Context of the build which should be set to '/'.  You can now click through the rest of the selections and will end up with a fully populated trigger for the repository.

You can now trigger the first build and test cycle for the repository.  Once the repository has been built you can proceed to setting up a Kubernetes test cluster than can pull the image(s) from the repository as they are updated via git commits followed by a git push.

## Development and local image wrangling

In this use case the CI/CD based builds are performed using images that have been built within a Kubernetes cluster.  The first step in these pipelines is the creation of the images containing the needed build tooling and the source code creating an entirely encapsulated environment.  To build images within Kubernetes a duat tool, git-watch, is being used.  This tool monitors a github repository and triggers on pushed commits to corral the source code for the  software to be built and then a Makisu image creation pod within a Kubernetes cluster.  The Makisu build then pushes build images to a user nominated repository which becomes the triggering point for the CI/CD downstream steps.

```
export KUBE_CONFIG=~/.kube/microk8s.config
export KUBECONFIG=~/.kube/microk8s.config

export Registry=`cat ../../registry_local.yaml`

git-watch -v --job-template ../../ci_containerize.yaml https://github.com/leaf-ai/studio-go-runner.git^master
```

# Continuous Integration

The presence of the quay.io image repository allows a suitably configured Kubernetes cluster to query for build images and to use these for testing and integration.

The studio go runner standalone build image can be used within a go runner deployment to perform testing and validation against a live minio (s3 server) and a RabbitMQ (queue server) instances deployed within a single Kubernetes namespace.  The definition of the deployment is stored within the source code repository, in the file ci\_keel.yaml.

The build deployment contains an annotated deployment of the build image that when deployed concurrently with keel can react to freshly created build images to cycle automatically through build, test, deploy image cycles.

Keel is documented at https://keel.sh/, installation instruction can also be found there, https://keel.sh/guide/installation.html.  Once deploy keel can be left to run as a background service observing Kubernetes deployments that contain annotations it is designed to react to.  Keel will watch for changes to image repositories that Deployments have annotations for and will automatically upgrade the Deployment pods as new images are seen.

The studio go runner ci\_keel.yaml pods use Kubernetes annotations for the studio go runner istandalobe build deployment that the user should look into and configure to select the branches for which they want to watch and perform tests and releases for.  The keel labels within the ci\_keel.yaml file dictate under what circumstances the keel server will trigger a new pod for the build and test to be created in response to the reference build image changing as git commit and push operations are performed.  Information about these labels can be found at, https://keel.sh/v1/guide/documentation.html#Policies.

The commands that you might performed to this point in order to deploy keel into an existing Kubernetes deploy might well appear as follows:

```
mkdir -p ~/project/src/github.com/keel-hq
cd ~/project/src/github.com/keel-hq
git clone https://github.com/keel-hq/keel.git
cd keel
kubectl create -f deployment-rbac.yaml
mkdir -p ~/project/src/github.com/leaf-ai
cd ~/project/src/github.com/leaf-ai
git clone https://github.com/leaf-ai/studio-go-runner.git
cd studio-go-runner
git checkout [branch name]
# Follow the instructions for setting up the Prerequisites for compilation in the main README.md file
```

The next step would be to edit the ci\_keel.yaml file to reflect the branch name on which the development is being performed or the release prepared, and then deploy the integration stack.

```
stencil -input ci_keel.yaml -values Registry=xxx,Namespace=ci-go-runner | kubectl apply -f -
```

This will deploy a stack capable of builds and testing.  As a build finishes the stack will scale down the dependencies it uses for queuing and storage and will keep the build container alive so that logs can be examined.  The build activities will disable container upgrades while the build is running and will then open for upgrades once the build steps have completed to prevent premature termination.  When the build, and test has completed and pushed commits have been seen for the code base then the pod will be shutdown for the latest build and a new pod created.

If the env variable GITHUB\_TOKEN is present when deploying an integration stack it will be placed as a Kubernetes secret into the integration stack.  If the secret is present then upon successful build and test cycles the running container will attempt to create and deploy a release using the github release pages.

The Registry value, xxx, is used to pass your docker hub username, and password to keel orchestrated containers and the release image builder, Makisu, using a kubernetes secret.  An example of how to set this value is included in the next section, continue on for more details.  Currently only dockerhub is supported for pushing release images to.

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
export K8S_NAMESPACE=ci-go-runner-`petname`
stencil -input ci_keel.yaml -values Registry=${Registry},Namespace=$K8S_NAMESPACE | kubectl apply -f -

export K8S_POD_NAME=`kubectl --namespace=$K8S_NAMESPACE get pods -o json | jq '.items[].metadata.name | select ( startswith("build-"))' --raw-output`
kubectl --namespace $K8S_NAMESPACE logs -f $K8S_POD_NAME
```

or

```
export Registry=`cat registry.yaml`
stencil -input ci_keel.yaml -values Namespace=ci-go-runner-`petname`| kubectl apply -f -
export K8S_NAMESPACE=`kubectl get ns -o json | jq --raw-output '.items[] | select(.metadata.name | startswith("ci-go-runner-")) | .metadata.name'`

export K8S_POD_NAME=`kubectl --namespace=$K8S_NAMESPACE get pods -o json | jq '.items[].metadata.name | select ( startswith("build-"))' --raw-output`
kubectl --namespace $K8S_NAMESPACE logs -f $K8S_POD_NAME
```

# Locally deploy keel testing and CI

These instructions will be useful to those using a locally deployed Kubernetes distribution such as microk8s.  If you wish to use microk8s you should first deploy using the workstations instructions found in this souyrce code repository at docs/workstation.md.  You can then return to this section for further information on deploying the keel based CI/CD within your microk8s environment.

In the case that a test of a locally pushed docker image is needed you can build your image locally and then when the build.sh is run it will do a docker push to a microk8s cluster instance running on your workstation or laptop.  In order for the keel deployment to select the locally hosted image registry you set the Image variable for stencil to substitute into the ci\_keel.yaml file.

When the release features are used the CI/CD system will make use of the Makisu image builder, authored by Uber.  Makisu allows docker containers to build images entirely within an existing container with no specialized dependencies and also without needing dind (docker in docker), or access to a docker server socket.

```
./build.sh
stencil -input ci_keel.yaml -values Registry=${Registry},Image=localhost:32000/leafai/studio-go-runner-standalone-build:${GIT_BRANCH},Namespace=ci-go-runner-`petname`| kubectl apply -f -
```
