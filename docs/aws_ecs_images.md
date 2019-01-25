# studio-go-runner AWS ECS offboard registry support

This document contains notes and information that might be of value to administrators wanting to access AWS docker image registries.

This document is a work in progress for storing and using AWS secrets and tokens.  These techniques are intended for use in walled garden or fully RBAC managed Kubernetes clusters.

## Section

When using images from AWS the best practice calls for using a private AWS registry.  To do this AWS credentials need to be used to refresh a token on a regular basis and to store it for use by the cluster when pulling images during upgrades and the like.  To do this a Kubernetes cron job should be created much like the following:

```
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: studioml-go-runner-ecr-env
data:
  SECRET_NAME: studioml-go-docker-key
  EMAIL: karlmutch@gmail.com
  AWS_ACCOUNT: 999999999999
  AWS_DEFAULT_REGION: us-west-2
  AWS_ACCESS_KEY_ID: AnAccessKeyIdThatIsVerySecret
  AWS_SECRET_ACCESS_KEY: "ALongKeyThatIsVerySecret"
---
apiVersion: batch/v1beta1
kind: CronJob
metadata:
  annotations:
  name: studioml-go-runner-ecr-cred
spec:
  concurrencyPolicy: Allow
  failedJobsHistoryLimit: 1
  jobTemplate:
    metadata:
      creationTimestamp: null
    spec:
      template:
        metadata:
          creationTimestamp: null
        spec:
          containers:
          - command:
            - /bin/sh
            - -c
            - |-
              TOKEN=`aws ecr get-login --region ${AWS_DEFAULT_REGION} --registry-ids ${AWS_ACCOUNT} | cut -d' ' -f6`
              kubectl delete secret --ignore-not-found $SECRET_NAME
              kubectl create secret docker-registry $SECRET_NAME \
              --docker-server=https://${AWS_ACCOUNT}.dkr.ecr.${AWS_DEFAULT_REGION}.amazonaws.com \
              --docker-username=AWS \
              --docker-password="${TOKEN}" \
              --docker-email="${EMAIL}"
              kubectl patch serviceaccount default -p '{"imagePullSecrets":[{"name":"'$SECRET_NAME'"}]}'
            image: odaniait/aws-kubectl:latest
            imagePullPolicy: IfNotPresent
            name: studioml-go-runner-ecr-cred
            envFrom:
            - configMapRef:
                name: studioml-go-runner-ecr-env
            resources: {}
            securityContext:
              capabilities: {}
            terminationMessagePath: /dev/termination-log
            terminationMessagePolicy: File
          dnsPolicy: Default
          hostNetwork: true
          restartPolicy: Never
          schedulerName: default-scheduler
          securityContext: {}
          terminationGracePeriodSeconds: 30
  schedule: 0 */6 * * *
  successfulJobsHistoryLimit: 3
  suspend: false
```

The Deployment specification you use for the runner is then augmented with the following fragments to enable image pull secrets and to refer to your private AWS image repository.

```
piVersion: apps/v1beta2
kind: Deployment
metadata:
...
spec:
 template:
   spec:
      imagePullSecrets:
        - name: studioml-go-docker-key
      containers:
...
        image: ${AWS_ACCOUNT}.dkr.ecr.${AWS_DEFAULT_REGION}.amazonaws.com/leafai/studio-go-runner/runner:${VERSION}
        imagePullPolicy: Always
...
```

To add the cronjob and start it the following comamnd would be used:

```
kubectl -f ... apply
kubectl create job --from=cronjob/studioml-go-runner-ecr-cred start
```

