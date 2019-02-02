# Continuous Integration Setup

This document describes setting up a CI pipline that can be used to prepare releases for studio go runner.

studio go runner is designed to run in resource intensive environments using GPU enabled machines and so providing a free hosted pipeline for CI/CD is cost prohibitive. As an alternative parties interested in studio go runner can make use of quay.io hosted images built automatically and are then pulled into a test and integration downstream Kubernetes provisioned cluster they own.  This allows testing to be done using the CI pipeline on both local laptops, workstations and in cloud or data center environments.

This document contains instructions that can be used for hardware configurations that individual users to large scale enterprises can use without incuring monthly charges from third party providers.  These instructions first detail how a quay.io trigger can be setup to trigger builds on github commits.  Instructions then detail how to make use of Keel, https://keel.sh/, to pull CI images into a cluster and run the pipeline.

Optional GITHUB_TOKEN secrets are added to the cluster

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
