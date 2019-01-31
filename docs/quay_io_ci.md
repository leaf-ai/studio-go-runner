# Continuous Integration Setup

This document describes setting up a CI pipline that can be used to prepare releases for studio go runner.

studio go runner is designed to run in resource intensive environments using GPU enabled machines and so providing a free hosted pipeline for CI/CD is cost prohibitive. As an alternative parties interested in studio go runner can make use of quay.io hosted images built automatically to then trigger test and integration downstream in their own Kubernetes provisioned clusters.

This document contains instructions detailing how a quay.io and private Kubernetes cluster can be created using the same method that the project maintainers utilize.

stencil -input ci_k8s.yaml | kubectl apply -f -
