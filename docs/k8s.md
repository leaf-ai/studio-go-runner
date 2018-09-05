# studio-go-runner Kubernetes features

This document describes features support by the studioml go runner (runner) that are supported for generic Kubernetes installations.

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
