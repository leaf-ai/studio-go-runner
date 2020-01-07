# Tekton based CI

The studio go runner project is current experimenting with a variety of CI pipeline tools in order to reduce the complexity of building and releasing the offering.

Tekton has a number of desirable characteristics for this project.  The runner pipeline needs to be buildable and deployable on hardware that is typically fairly expensive to source on monthly plans offered by CI SaaS vendors.  Tekton is thought to be a good fit as it can be deployed using generic Kubernetes clusters on traditional PC equipment with GPU cards and therefore offers a solution across the varied needs of its diverse users.  Tekton also offers a way of generating container images within Kubernetes without concerning developers with a arduous installation process needed for users of tooling such as Kaniko or Makisu.

This document details the instructions and progress on experimenting with Tekton.

This document make the assumtion that the microk8s Kubernetes distribution is being used.  microk8s is an Ubuntu distribution of Kubernetes that is designed for individual developer workstations.  To make use of kubectl in other contexts drop the 'microk8s.' prefix on the commands documented in this document.

# Audience

This document is targetted at devops roles for people with a working knowledge of Kubernetes and and container oriented development and releases.

## Installation

Tekton can be deployed using the instructions found at https://github.com/tektoncd/pipeline/blob/master/docs/install.md.

A CLI is available for Tekton and can be installed using the following, https://github.com/tektoncd/cli#getting-started.

In order to configure sharing between pipelines an S3 server can be configured, see instructions at https://github.com/tektoncd/pipeline/blob/master/docs/install.md#configuring-tekton-pipelines.  S3 is considered the best option in our case due to being an independent state store that can also be configured as an on-premises store using minio.

An example of using S3 configurations can be found in the tekton/storage.yaml file of this present github repository and could be applied using the stencil tool found at, https://github.com/karlmutch/duat/releases/download/0.11.6/stencil-linux-amd64

```
microk8s.kubectl apply -f <(stencil < tekton/storage.yaml)
```

Pay special attention to the requirement that the S3 storage bucket should reside in the us-east-1 zone due to internal tekton tooling.

Be sure to go through the tutorial(s) provided by Tekton to gain an appreciation of its features, https://github.com/tektoncd/pipeline/blob/master/docs/tutorial.md.


