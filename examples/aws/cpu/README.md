# Quick introduction to using AWS volumes during manual testing and exercises

This section gives the briefest of overviews for standing up a single CPU runner cluster, with optional encryption support.

<!--ts-->
<!--te-->

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
wget -O stencil https://github.com/karlmutch/duat/releases/download/0.13.0/stencil-linux-amd64
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

3. Generate secrets used to encrypt messages

Further information can in found in the [../../docs/message_privacy.md](../../docs/message_privacy.md) documentation.

```
echo -n "PassPhrase" > secret_phrase
ssh-keygen -t rsa -b 4096 -f studioml_message -C "Message Encryption Key" -N "PassPhrase"
ssh-keygen -f studioml_message.pub -e -m PEM > studioml_message.pub.pem
cp studioml_message studioml_message.pem
ssh-keygen -f studioml_message.pem -e -m PEM -p -P "PassPhrase" -N "PassPhrase"
kubectl create secret generic studioml-runner-key-secret --from-file=ssh-privatekey=studioml_message.pem --from-file=ssh-publickey=studioml_message.pub.pem
kubectl create secret generic studioml-runner-passphrase-secret --from-file=ssh-passphrase=secret_phrase
```

4. Deploy the runner

stencil < deployment.yaml | kubectl apply -f -

5. Run a studioml experiment using the python StudioML client

```
aws s3api create-bucket --bucket $USER-cpu-example-metadata --region $AWS_REGION --create-bucket-configuration LocationConstraint=$AWS_REGION
aws s3api create-bucket --bucket $USER-cpu-example-data --region $AWS_REGION --create-bucket-configuration LocationConstraint=$AWS_REGION

SECRET_CONFIG=`mktemp -p .`
stencil < studioml.config > $SECRET_CONFIG
virtualenv --python=python3.6 ./experiment
source ./experiment/bin/activate
pip install tensorflow==1.15.2
pip install studioml
SUBMIT_LOG=`mktemp -p .`
OUTPUT_LOG=`mktemp -p .`
studio run --config=$SECRET_CONFIG --lifetime=30m --max-duration=20m --gpus 0 --queue=sqs_${USER}_cpu_example  --force-git app.py >$SUBMIT_LOG 2>/dev/null
export EXPERIMENT_ID=`awk 'END {print $NF}' $SUBMIT_LOG`
rm $SUBMIT_LOG
EXIT_STRING="+ exit "
OUTPUT_DIR=`mktemp -d -p .`
for (( ; ; ))
    do
    sleep 5
    aws s3 cp s3://$USER-cpu-example-data/experiments/$EXPERIMENT_ID/output.tar $OUTPUT_DIR/$OUTPUT_LOG.tar 2>/dev/null || continue
    tar xvf $OUTPUT_DIR/$OUTPUT_LOG.tar -C $OUTPUT_DIR
    LAST_LINE=`tail -n 1 $OUTPUT_DIR/output`
    echo $LAST_LINE
    [[ $LAST_LINE == ${EXIT_STRING}* ]]; break
    rm $OUTPUT_DIR/output || true
    rm $OUTPUT_DIR/output.tar || true
done
rm $OUTPUT_DIR/output || true
rm $OUTPUT_DIR/$OUTPUT_LOG.tar || true
rmdir $OUTPUT_DIR
rm $OUTPUT_LOG
deactivate
rm -rf experiment
rm $SECRET_CONFIG

aws s3 rb s3://$USER-cpu-example-data --force
aws s3 rb s3://$USER-cpu-example-metadata --force

```

6. Clean up

```
kubectl delete -f examples/aws/cpu/deployment.yaml
aws ec2 delete-volume --volume-id=$AWS_VOLUME_ID
eksctl delete cluster --region=us-west-2 --name=$CLUSTER_NAME --wait
```

Copyright © 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
