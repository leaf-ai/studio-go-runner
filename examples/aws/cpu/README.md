# Quick introduction to using AWS volumes during manual testing and exercises

This section gives the briefest of overviews for standing up a single CPU runner cluster.

## Prerequisties

### Configuration

Have environment variables from the aws\_k8s.md instructions available including :

```
AWS_ACCESS_KEY
AWS_SECRET_ACCESS_KEY
```
You will also need the following additional environment variables with their values set appropriately:
```
export AWS_ACCOUNT=`aws sts get-caller-identity | jq ".Account" -r`
export AWS_REGION=us-west-2
export EMAIL=karl.mutch@cognizant.com
export AWS_IMAGE=docker.io/leafai/studio-go-runner:0.9.26-master-aaaagninkqg
```

### Software

Install a template processor based on the Go lang templater used by Kubernetes.

```
wget -O stencil https://github.com/karlmutch/duat/releases/download/0.12.1/stencil-linux-amd64
chmod +x stencil
```

## Steps

1. Start the cluster

The cluster is started with an EC2 volume that will be mounted by the runner pod.  This works around the issue with the size of the docker image.

```
export CLUSTER_NAME=test-eks
eksctl create cluster --name $CLUSTER_NAME --region $AWS_REGION --nodegroup-name $CLUSTER_NAME-workers --node-type t3a.2xlarge --nodes 1 --nodes-min 1 --nodes-max 3 --ssh-access --ssh-public-key ~/.ssh/id_rsa.pub --managed

export ZONE=`kubectl get nodes -o jsonpath="{.items[0].metadata.labels['failure-domain\.beta\.kubernetes\.io/zone']}"`
export AWS_VOLUME_ID=`aws ec2 create-volume --availability-zone $ZONE --size 60 --volume-type gp2 --output json | jq '.VolumeId' -r`
```

2. Ensure that the AWS secrets are loaded for SQS queues

```
aws_sqs_cred=`cat ~/.aws/credentials | base64 -w 0`
aws_sqs_config=`cat ~/.aws/config | base64 -w 0`
kubectl apply -f <(cat <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: studioml-runner-aws-sqs
type: Opaque
data:
  credentials: $aws_sqs_cred
  config: $aws_sqs_config
EOF
)
```

3. Deploy the runner

stencil < deployment.yaml | kubectl apply -f -

3. Clean up

kubectl delete -f examples/aws/cpu/deployment.yaml
aws ec2 delete-volume --volume-id=$AWS_VOLUME_ID
eksctl delete cluster --region=us-west-2 --name=$CLUSTER_NAME --wait
