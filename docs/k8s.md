# studio-go-runner Kubernetes features

This document describes features support by the studioml go runner (runner) that are supported for generic Kubernetes installations.

Detection
Config Map discovery
studioml-${HOSTNAME}
```
---
apiVersion: v1
kind: ConfigMap
metadata:
 name: studioml-studioml-go-runner-deployment-67f5c55587-smb8q
 data:
  STATE: "Running"
```
States
Running, DrainAndTerminate, DrainAndSuspend

Other states such as a hard abort, or a hard restart can be done using Kubernetes and are not an application state
