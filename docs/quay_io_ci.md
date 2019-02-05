# Continuous Integration Setup

This document describes setting up a CI pipline that can be used to prepare releases for studio go runner.

studio go runner is designed to run in resource intensive environments using GPU enabled machines and so providing a free hosted pipeline for CI/CD is cost prohibitive. As an alternative, parties interested in studio go runner can make use of quay.io hosted images built automatically on github commit triggers to then trigger their own downstream build, test and deploy automation.  Downstream automation can be hosted on a self provisioned Kubernetes provisioned cluster either within the cloud or on private infrastructure.  This allows testing to be done using the CI pipeline on both local laptops, workstations and in cloud or data center environments.

This document contains instructions that can be used for hardware configurations that individual users to large scale enterprises can use without incuring monthly charges from third party providers.  These instructions first detail how a quay.io trigger can be setup to trigger builds on github commits.  Instructions then detail how to make use of Keel, https://keel.sh/, to pull CI images into a cluster and run the pipeline.

# A word about privacy

Many of the services that provide image hosting use Single Sign On and credentials management with your source code control platform of choice.  As a consequence of this often these services will gain access to any and all repositories private or otherwise that you might have access to within your account.  In order to preserve privacy and maintain fine grained control over the visibility of your private repositories it is recommended that when using quay.io and other services that you create a service account that has the minimal level of access to repositories as nessasary to implement your CI/CD features.

# CI Image building

The studio go runner project uses Docker images to encapsulate builds within an immutable archive format.  Using internet accessible hosted registries it is possible to configure a registry to actively build an image from the git repository at that commit and to then host the resulting image.  A number of internet registries offer hosting for open source projects for free, and also offer paid hosted plans for users requiring privacy.  These instructions give a summary of what needs to be done in order to use the quay.io service to provision an image repository that auto-builds images from the studio go runner project.

The first step is to create or login to an account on quay.io.  When creating an account on quay.io it is best to ensure before starting that you have a browser window open to giuthub.com using the account that you wish to use for accessing code on github to prevent any unintended accesses to private repositories.  As you create the account on you can choose to link it automatically to github granting application access from quay to your github authorized applications.  This is needed in order that quay can poll your projects for any changes in order to trigger image building.

Having logged in you can now create a repository using the label at the top right corner of your web page underneath the account related drop down menu.

The first screen will allow you to specify tgar you wish to create an image repository and assign it a name, also set the visibility to public, and to 'Link to a GitHub Repository Push', this indicates that any push of a commit or tag will result in a container build being triggered.

Pushing the next button will then cause the browser to request github to authorize access from quay to github and will prompt you to allow this authorization to be setup for future interactions between the two platform.  Again, be sure you are assuming the role of the most recently logged in github user and that the one being authorized is the one you intend to allow Quay to obtain access to.

After the auhtorization is enabled the next web page is displayed which allows the organization and account to be choosen from which the image will be built.  Step through the next two screen to drill down to the repository that will be used and then push the continue button.

You can then specify the branch(es) that can then be used for the builds to meet you own needs.  Pushing con tinue will then allow you to select the Dockerfile that will act as your source for the new image.  When using studio go runner a Dockerfile called Dockerfile_standalone is versioned in the source code repository that will allow a fully standalone container to be created that can be perform the entire build, test, release life cycle for the software.  usign a slash indicates the top level of the go runner repo.

Using continue will then prompt for the Context of the build which should be set to '/'.  You can now click through the rest of the selections and will end up with a fully populated trigger for the repository.

You can now trigger the first build and test cycle for the repository.  Once the repository has been built you can proceed to setting up a Kubernetes test cluster than can pull the image(s) from the repository as they are updated via git commits followed by a git push.

# Continuous Integration

The presence of a publically accesible repository allows a suitably configured Kubernetes cluster to query for the presence of build images for testing and integration.

The studio go runner standalone build image can be used within a go runner deployment to perform testing and validation against a live minio(s3 server) and a RabbitMQ (queue server) instances deployed within a single Kubernetes namespace.  The definition of the deployment is stored within the source code repository, as ci_keel.yaml.

The build deployment contains an annotated deployment of the build image that when deployed concurrently with http://keel.sh/ can react to freshly created build images to cycle through build, deploy, test cycles automatically.

Keel is documented at https://keel.sh/, installation instruction can also be found there, https://keel.sh/guide/installation.html.  Once deploy keel can be left to run as a background service observing Kubernetes deployments that contain annotations it is designed to react to.  Keel will watch for changes to image repositories that Deployments have annotations for and will automatically upgrade the Deployment pods as new images are seen.

The studio go runner ci_keel.yaml contains annotations for a studio go runner Deployment that the user should look into and configure to select the branches for which they want to watch and perform tests and releases for.  The keel labels within the ci_keel.yaml file dictate under what circumstances the keel server will trigger a new pod for the build and test to be created in response to the reference build image changing as git commit and push operations are performed.  Information about these labels can be found at, https://keel.sh/v1/guide/documentation.html#Policies.

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

The next step would be to edit the ci_keel.yaml file to reflect the branch name on which the development is being performed or the release prepared, and then deploy the integration stack.

```
stencil -input ci_keel.yaml -values Namespace=ci-go-runner | kubectl apply -f -
```

This will deploy a stack capable of builds and testing.  As a build finishes the stack will scale down the dependencies it uses for queuing and storage and will keep the build container alive so that logs can be examined.  The build activities will disable container upgrades while the build is running and will then open for upgrades once the build steps have completed to prevent premature termination.  When the build, and test has completed and pushed commits have been seen for the code base then the pod will be shutdown for the latest build and a new pod created.

When deploying an integration stack it is possible to specify a GithubToken for performing releases.  If the token is present as a Kubernetes secret then upon successful build and test cycles the running container will attempt to create and deploy a release using the github release pages.

OPtional GITHUB_TOKEN secrets are added to the cluster

Annotations updated via stencil with gitHash etc and also with desired regular expression or keel semver policy
namespace is generated and used for the bootstrapped build
stencil -input ci_keel.yaml | kubectl apply -f -
git commit and push to start things rolling
Keel repo polling triggers build

built container in build pod removes itself from keel using Kubernetes preStartHook by renaming annotations
```
Using downward API
metadata.annotations['myannotation']
```

build pod starts
new namespace generated for next listener
```
github.com/docker/docker/pkg/namesgenerator
Loop creating namespace with uuid annotation and then validating we owned it
```

container used the included ci_keel and injects the annotations from itself to create the next listening deployment
```
stencil with variables in a file for all annotations now renamed for their real keys
```

new namspace with deployment using ci_keel.yaml is dispatched
build starts in our now liberated namespace

build finishes
set ReplicationControllers and deployment .spec.replicas to 0
```
kubectl scale --namespace build-test-k8s-local --replicas=0 deployment/minio-deployment
kubectl scale --namespace build-test-k8s-local --replicas=0 rc/rabbitmq-controller
```

and the build then sits until such time as we decide on a policy for self destruction like push results back to github, at which point we dispose of the unique namespace used for the build
