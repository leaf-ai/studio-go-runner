# studio-go-runner Kubernetes features

This document describes features support by the studioml go runner (runner) that are supported for generic Kubernetes installations.

## Using k8s build and test

The runner supports a full build mode which can be used to perform local or remote builds without needing a local developer environment configured.  The Dockerfile_full image specification file contains the directives to do this.

The runner does have a build mode which mounts code from a developers work environment into a running container within the default Dockerfile.

When build.sh is used to perform local developer builds a container is also produced tagged as $azure_registry_name.azurecr.io/sentient.ai/studio-go-runner/standalone-build.  This container when built will be pushed to azure and AWS docker image registries if the appropriate cloud environment tooling is available or environment variables are set, $azure\_registry\_name, and for AWS a default account configcurrentured and ECR login activated.  The image when run will access the github source repo and will build and test the code for the master branch.

If you wish to test the Kubernetes features create a cluster with a single agent node that uses the cloud providers GPU host type.  Then create a secret to enable access from your cluster to the your AWS or Azure registry.

```
kubectl create secret docker-registry studioml-go-docker-key --docker-server=studio-repo.azurecr.io --docker-username=studio-repo --docker-password=long-hash-value --docker-email=karlmutch@gmail.com
```

You can then run the build in an ad-hoc fashion using a command such as the following:

```
kubectl run --image=studio-repo.azurecr.io/sentient.ai/studio-go-runner/standalone-build --attach --requests="nvidia.com/gpu=1" --limits="nvidia.com/gpu=1" build
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
kubectl run --image=quotaworkaround001.azurecr.io/sentient.ai/studio-go-runner/standalone-build --attach --requests="nvidia.com/gpu=1" --limits="nvidia.com/gpu=1" build

If you don't see a command prompt, try pressing enter.
Branch feature/137_service_management set up to track remote branch feature/137_service_management from origin.
Switched to a new branch 'feature/137_service_management'
Warning: CUDA not supported on this platform stack="[cuda_nosupport.go:30 cuda.go:70]"
=== RUN   TestK8sConfig
--- PASS: TestK8sConfig (0.00s)
=== RUN   TestStrawMan
--- PASS: TestStrawMan (0.00s)
PASS
ok      github.com/SentientTechnologies/studio-go-runner/internal/runner        0.011s
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
Running, DrainAndTerminate, DrainAndSuspend
```

Other states such as a hard abort, or a hard restart can be done using Kubernetes and are not an application state

### Security requirements

```
kubectl create clusterrolebinding default-cluster-admin --clusterrole=cluster-admin --serviceaccount=default:default
```

# k8s and GPU based testing

kubectl run --image=quotaworkaround001.azurecr.io/sentient.ai/studio-go-runner/build --requests="nvidia.com/gpu=1" --limits="nvidia.com/gpu=1" build
kubectl run --image=quotaworkaround001.azurecr.io/sentient.ai/studio-go-runner/build --attach --requests="nvidia.com/gpu=1" --limits="nvidia.com/gpu=1" build --command sleep 100m

kubectl delete deployment build
