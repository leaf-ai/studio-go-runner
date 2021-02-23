# studio-go-runner AWS support

This document details the installation of the studio go runner within an Azure hosted Kubernetes cluster.  After completing the Kubernetes installation using these instructions please return to the main README.md file to continue.

If you are interested in using CPU deployments with attached EBS volumes the [README at examples/aws/cpu/README.md](examples/aws/cpu/README.md) will be of interest.

# Prerequisites

* Install and configure the AWS Command Line Interface (AWS CLI):
    * Install the [AWS Command Line Interface](https://docs.aws.amazon.com/cli/latest/userguide/install-cliv2-linux.html)
    * Configure the AWS CLI using the command: `aws configure`.
    * Enter credentials ([Access Key ID and Secret Access Key](https://docs.aws.amazon.com/general/latest/gr/aws-sec-cred-types.html#access-keys-and-secret-access-keys)).
    * Enter the Region and other options.
    * Install the jq utility for post procesisng AWS CLI output
* Install [eksctl](https://github.com/weaveworks/eksctl).
* Load the AWS SQS Credentials
* Deploy the runner

## Install eksctl (AWS only)

If you are using azure or GCP then options such as acs-engine, and skaffold are natively supported by the cloud vendors.  These tools are also readily customizable, and maintained and so these are recommended.

For AWS the eksctl tool is now considered the official tool for the EKS CLI.  iA full set of instructions for the installation of eksctl can be found at,https://docs.aws.amazon.com/eks/latest/userguide/getting-started-eksctl.html. In brief form eksctl can be installed using the following steps:

```shell
pip install awscli --upgrade --user
curl --silent --location "https://github.com/weaveworks/eksctl/releases/latest/download/eksctl_$(uname -s)_amd64.tar.gz" | tar xz -C /tmp
sudo mv /tmp/eksctl /usr/local/bin
sudo apt-get install jq
```

One requirement of using eksctl is that you must first subscribe to the AMI that will be used with your GPU EC2 instances.  The subscription can be found at, https://aws.amazon.com/marketplace/pp/B07GRHFXGM.


## AWS Cloud support for Kubernetes 1.18.x and GPU

This section discusses the use of eksctl to provision a working k8s cluster onto which the gpu runner can be deployed.

The use of AWS EC2 machines requires that the AWS account has had an EC2 key Pair imported from your administration machine, or created in order that machines created using eksctl can be accessed.  More information can be found at https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-key-pairs.html.

In order to make use of StudioML environment variable based templates you should export the AWS environment variables.  While doing this you should also synchronize your system clock as this is a common source of authentication issues with AWS.  

<pre><code><b>export AWS_ACCOUNT=`aws sts get-caller-identity --query Account --output text`
aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin $AWS_ACCOUNT.dkr.ecr.us-west-2.amazonaws.com
export AWS_ACCESS_KEY=xxx
export AWS_SECRET_ACCESS_KEY=xxx
export AWS_DEFAULT_REGION=xxx
sudo ntpdate ntp.ubuntu.com
</b></code></pre>

<pre><code><b>
export KUBECONFIG=~/.kube/config
export AWS_CLUSTER_NAME=test-eks
eksctl create cluster --name $AWS_CLUSTER_NAME --region us-west-2 --nodegroup-name $AWS_CLUSTER_NAME --node-type p2.xlarge  --nodes 1 --nodes-min 1 --nodes-max 3 --ssh-access --ssh-public-key ~/.ssh/id_rsa.pub --managed</b>
[ℹ]  eksctl version 0.35.0
[ℹ]  using region us-west-2
[ℹ]  setting availability zones to [us-west-2b us-west-2a us-west-2d]
[ℹ]  subnets for us-west-2b - public:192.168.0.0/19 private:192.168.96.0/19
[ℹ]  subnets for us-west-2a - public:192.168.32.0/19 private:192.168.128.0/19
[ℹ]  subnets for us-west-2d - public:192.168.64.0/19 private:192.168.160.0/19
[ℹ]  using SSH public key "/home/kmutch/.ssh/id_rsa.pub" as "eksctl-test-eks-nodegroup-test-eks-be:07:a0:27:44:d8:27:04:c2:ba:28:fa:8c:47:7f:09"
[ℹ]  using Kubernetes version 1.18
[ℹ]  creating EKS cluster "test-eks" in "us-west-2" region with managed nodes
[ℹ]  will create 2 separate CloudFormation stacks for cluster itself and the initial managed nodegroup
[ℹ]  if you encounter any issues, check CloudFormation console or try 'eksctl utils describe-stacks --region=us-west-2 --cluster=test-eks'
[ℹ]  CloudWatch logging will not be enabled for cluster "test-eks" in "us-west-2"
[ℹ]  you can enable it with 'eksctl utils update-cluster-logging --enable-types={SPECIFY-YOUR-LOG-TYPES-HERE (e.g. all)} --region=us-west-2 --cluster=test-eks'
[ℹ]  Kubernetes API endpoint access will use default of {publicAccess=true, privateAccess=false} for cluster "test-eks" in "us-west-2"
[ℹ]  2 sequential tasks: { create cluster control plane "test-eks", 3 sequential sub-tasks: { no tasks, create addons, create managed nodegroup "test-eks" } }
[ℹ]  building cluster stack "eksctl-test-eks-cluster"
[ℹ]  deploying stack "eksctl-test-eks-cluster"
[ℹ]  building managed nodegroup stack "eksctl-test-eks-nodegroup-test-eks"
[ℹ]  deploying stack "eksctl-test-eks-nodegroup-test-eks"
[ℹ]  waiting for the control plane availability...
[✔]  saved kubeconfig as "/home/kmutch/.kube/microk8s.config"
[ℹ]  no tasks
[✔]  all EKS cluster resources for "test-eks" have been created
[ℹ]  nodegroup "test-eks" has 1 node(s)
[ℹ]  node "ip-192-168-22-79.us-west-2.compute.internal" is ready
[ℹ]  waiting for at least 1 node(s) to become ready in "test-eks"
[ℹ]  nodegroup "test-eks" has 1 node(s)
[ℹ]  node "ip-192-168-22-79.us-west-2.compute.internal" is ready
[ℹ]  kubectl command should work with "/home/kmutch/.kube/config", try 'kubectl --kubeconfig=/home/kmutch/.kube/config get nodes'
[✔]  EKS cluster "test-eks" in "us-west-2" region is ready
</code></pre>

eksctl is written in Go uses CloudFormation internally and supports the use of YAML resources to define deployments, more information can be found at https://eksctl.io/.

When creating a cluster the credentials will be loaded into your ~/.kube/config file automatically.  When using the AWS service oriented method of deployment the normally visible master will not be displayed as a node.

## GPU Setup

In order to activate GPU support within the workers a daemon set instance needs to be created that will mediate between the kubernetes plugin and the GPU resources available to pods, as shown in the following command.

<pre><code><b>
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/1.0.0-beta6/nvidia-device-plugin.yml</b>
daemonset.apps/nvidia-device-plugin-daemonset created
</code></pre>

Machines when first started will have an allocatable resource named nvidia.com/gpu.  When this resource flips from 0 to 1 the machine has become available for GPU work.  The plugin yaml added will cause a container to be bootstrapped into new nodes to perform the installation of the drivers etc.

<pre><code><b>
kubectl get nodes "-o=custom-columns=NAME:.metadata.name,GPU:.status.allocatable.nvidia\.com/gpu"</b>
NAME                                         GPU
ip-192-168-5-16.us-west-2.compute.internal   1
</code></pre>

## GPU Testing

A test pod for validating the GPU functionality can be created using the following commands:

```
$ cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: tf-gpu
spec:
  containers:
  - name: gpu
    image: 763104351884.dkr.ecr.us-west-2.amazonaws.com/tensorflow-training:2.3.1-gpu-py37-cu110-ubuntu18.04
    imagePullPolicy: IfNotPresent
    command: ["/bin/sh", "-c"]
    args: ["sleep 10000"]
    resources:
      limits:
        memory: 1024Mi
        # ^ Set memory in case default limits are set low
        nvidia.com/gpu: 1 # requesting 1 GPUs
        # ^ For Legacy Accelerators mode this key must be renamed
        #   'alpha.kubernetes.io/nvidia-gpu'
  tolerations:
  # This toleration will allow the gpu hook to run anywhere
  #   By default this is permissive in case you have tainted your GPU nodes.
  - operator: "Exists"
EOF
```

Once the pod is in a running state you should be able to test the access to the GPU cards using the following commands:

<pre><code><b>
kubectl get pods</b>
NAME     READY   STATUS    RESTARTS   AGE
tf-gpu   1/1     Running   0          2m31s
 <b>kubectl exec -it tf-gpu -- \
  python -c 'from tensorflow.python.client import device_lib; print(device_lib.list_local_devices())'</b>
WARNING:tensorflow:From /usr/local/lib/python3.6/dist-packages/tensorflow_core/__init__.py:1467: The name tf.estimator.inputs is deprecated. Please use tf.compat.v1.estimator.inputs instead.

2020-04-02 19:53:04.846974: I tensorflow/core/platform/profile_utils/cpu_utils.cc:94] CPU Frequency: 2300070000 Hz
2020-04-02 19:53:04.847631: I tensorflow/compiler/xla/service/service.cc:168] XLA service 0x47a9050 initialized for platform Host (this does not guarantee that XLA will be used). Devices:
2020-04-02 19:53:04.847672: I tensorflow/compiler/xla/service/service.cc:176]   StreamExecutor device (0): Host, Default Version
2020-04-02 19:53:04.851171: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcuda.so.1
2020-04-02 19:53:05.074667: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:983] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2020-04-02 19:53:05.075725: I tensorflow/compiler/xla/service/service.cc:168] XLA service 0x4870840 initialized for platform CUDA (this does not guarantee that XLA will be used). Devices:
2020-04-02 19:53:05.075757: I tensorflow/compiler/xla/service/service.cc:176]   StreamExecutor device (0): Tesla K80, Compute Capability 3.7
2020-04-02 19:53:05.077045: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:983] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2020-04-02 19:53:05.077866: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1639] Found device 0 with properties:
name: Tesla K80 major: 3 minor: 7 memoryClockRate(GHz): 0.8235
pciBusID: 0000:00:1e.0
2020-04-02 19:53:05.078377: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcudart.so.10.0
2020-04-02 19:53:05.080249: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcublas.so.10.0
2020-04-02 19:53:05.081941: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcufft.so.10.0
2020-04-02 19:53:05.082422: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcurand.so.10.0
2020-04-02 19:53:05.084606: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcusolver.so.10.0
2020-04-02 19:53:05.086207: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcusparse.so.10.0
2020-04-02 19:53:05.090706: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcudnn.so.7
2020-04-02 19:53:05.090908: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:983] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2020-04-02 19:53:05.091833: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:983] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2020-04-02 19:53:05.092591: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1767] Adding visible gpu devices: 0
2020-04-02 19:53:05.092655: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcudart.so.10.0
2020-04-02 19:53:05.094180: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1180] Device interconnect StreamExecutor with strength 1 edge matrix:
2020-04-02 19:53:05.094214: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1186]      0
2020-04-02 19:53:05.094237: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1199] 0:   N
2020-04-02 19:53:05.094439: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:983] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2020-04-02 19:53:05.095349: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:983] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2020-04-02 19:53:05.096185: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1325] Created TensorFlow device (/device:GPU:0 with 10805 MB memory) -> physical GPU (device: 0, name: Tesla K80, pci bus id: 0000:00:1e.0, compute capability: 3.7)
[name: "/device:CPU:0"
device_type: "CPU"
memory_limit: 268435456
locality {
}
incarnation: 15851552145019400091
, name: "/device:XLA_CPU:0"
device_type: "XLA_CPU"
memory_limit: 17179869184
locality {
}
incarnation: 589949818737926036
physical_device_desc: "device: XLA_CPU device"
, name: "/device:XLA_GPU:0"
device_type: "XLA_GPU"
memory_limit: 17179869184
locality {
}
incarnation: 1337920997684791636
physical_device_desc: "device: XLA_GPU device"
, name: "/device:GPU:0"
device_type: "GPU"
memory_limit: 11330115994
locality {
  bus_id: 1
  links {
  }
}
incarnation: 6377093002559650203
physical_device_desc: "device: 0, name: Tesla K80, pci bus id: 0000:00:1e.0, compute capability: 3.7"
]
<b>kubectl exec -it tf-gpu nvidia-smi</b>
Thu Apr  2 19:58:15 2020       
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 418.87.00    Driver Version: 418.87.00    CUDA Version: 10.1     |
|-------------------------------+----------------------+----------------------+
| GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp  Perf  Pwr:Usage/Cap|         Memory-Usage | GPU-Util  Compute M. |
|===============================+======================+======================|
|   0  Tesla K80           On   | 00000000:00:1E.0 Off |                    0 |
| N/A   44C    P8    27W / 149W |      0MiB / 11441MiB |      0%      Default |
+-------------------------------+----------------------+----------------------+
                                                                               
+-----------------------------------------------------------------------------+
| Processes:                                                       GPU Memory |
|  GPU       PID   Type   Process name                             Usage      |
|=============================================================================|
|  No running processes found                                                 |
+-----------------------------------------------------------------------------+

<b>kubectl delete pod tf-gpu</b>
pod "tf-gpu" deleted
</code></pre>

It is also possible to use the stock nvidia docker images to perform tests as well, for example:

```
$ cat << EOF | kubectl create -f -
apiVersion: v1
kind: Pod
metadata:
  name: nvidia-smi
spec:
  restartPolicy: OnFailure
  containers:
  - name: nvidia-smi
    image: nvidia/cuda:latest
    args:
    - "nvidia-smi"
    resources:
      limits:
        nvidia.com/gpu: 1
EOF
pod/nvidia-smi created
$ kubectl logs nvidia-smi
Thu Apr  2 20:03:44 2020
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 418.87.00    Driver Version: 418.87.00    CUDA Version: 10.1     |
|-------------------------------+----------------------+----------------------+
| GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp  Perf  Pwr:Usage/Cap|         Memory-Usage | GPU-Util  Compute M. |
|===============================+======================+======================|
|   0  Tesla K80           On   | 00000000:00:1E.0 Off |                    0 |
| N/A   44C    P8    27W / 149W |      0MiB / 11441MiB |      2%      Default |
+-------------------------------+----------------------+----------------------+

+-----------------------------------------------------------------------------+
| Processes:                                                       GPU Memory |
|  GPU       PID   Type   Process name                             Usage      |
|=============================================================================|
|  No running processes found                                                 |
+-----------------------------------------------------------------------------+
$ kubectl delete pod nvidia-smi
pod "nvidia-smi" deleted
```

## Load the AWS SQS Credentials

When using AWS deployments have the choice of making use of the AWS hosted RabbitMQ offering or using AWS SQS queuing.

### AWS RabbitMQ


Rabbit MQ is available within AWS as a managed service.  The message broker can be made publically access or to remain within the confines of your VPC, if you decide to make it publically accessible then you should modify the --no-publicly-accessible option. and can be configured within your AWS account using the following command:

```
$ export RMQ_BROKER=test-rmq
$ export RMQ_ADMIN_PASSWORD=admin_password
$ export RMQ_ADMIN_USER=admin
$ aws mq create-broker  --host-instance-type mq.m5.large --broker-name $RMQ_BROKER --engine-version 3.8.6 \
--deployment-mode SINGLE_INSTANCE --engine-type RABBITMQ  --no-publicly-accessible \
--tags "Owner=Karl Mutch" --users ConsoleAccess=true,Groups=administrator,Password=$RMQ_ADMIN_PASSWORD,Username=$RMQ_ADMIN_USER
$ export AWS_RMQ_ID=`aws mq list-brokers | jq '.BrokerSummaries[] | select(.BrokerName=="test-rmq") | .BrokerId' -r`
```

At this point it will take 5 or more minutes for the RMQ cluster to start so be sure to either check the AWS management console or run the `aws mq list-brokers` command checking to see when the broker has moved into the running state. Once the broker is running use the following commands to get the host details for the broker.

```
export RMQ_URL=`aws mq describe-broker --broker-id $AWS_RMQ_ID | jq -r ".BrokerInstances[0].Endpoints[0]"`
export RMQ_MANAGEMENT_URL=`aws mq describe-broker --broker-id $AWS_RMQ_ID | jq -r ".BrokerInstances[0].ConsoleURL"`

# extract the protocol
rmq_proto="$(echo $RMQ_URL | grep :// | sed -e's,^\(.*://\).*,\1,g')"

# remove the protocol -- updated
rmq_url=$(echo $RMQ_URL | sed -e s,$rmq_proto,,g)

# extract the user (if any)
ignore_user="$(echo $rmq_url | grep @ | cut -d@ -f1)"

# extract the host and port -- updated
rmq_hostport=$(echo $rmq_url | sed -e s,$ignore_user@,,g | cut -d/ -f1)

# by request host without port
rmq_host="$(echo $rmq_hostport | sed -e 's,:.*,,g')"
# by request - try to extract the port
rmq_port="$(echo $rmq_hostport | sed -e 's,^.*:,:,g' -e 's,.*:\([0-9]*\).*,\1,g' -e 's,[^0-9],,g')"

# extract the path (if any)
rmq_path="$(echo $rmq_url | grep / | cut -d/ -f2-)"
export RMQ_FULL_URL="$rmq_proto$RMQ_ADMIN_USER:$RMQ_ADMIN_PASSWORD@$rmq_hostport/$rmq_path"
export AMQP_URL=$RMQ_FULL_URL

# extract the protocol
rmq_proto="$(echo $RMQ_MANAGEMENT_URL | grep :// | sed -e's,^\(.*://\).*,\1,g')"

# remove the protocol -- updated
rmq_url=$(echo $RMQ_MANAGEMENT_URL | sed -e s,$rmq_proto,,g)

# extract the user (if any)
ignore_user="$(echo $rmq_url | grep @ | cut -d@ -f1)"

# extract the host and port -- updated
rmq_hostport=$(echo $rmq_url | sed -e s,$ignore_user@,,g | cut -d/ -f1)

# by request host without port
rmq_host="$(echo $rmq_hostport | sed -e 's,:.*,,g')"
# by request - try to extract the port
rmq_port="$(echo $rmq_hostport | sed -e 's,^.*:,:,g' -e 's,.*:\([0-9]*\).*,\1,g' -e 's,[^0-9],,g')"

# extract the path (if any)
rmq_path="$(echo $rmq_url | grep / | cut -d/ -f2-)"
export RMQ_ADMIN_URL="$rmq_proto$RMQ_ADMIN_USER:$RMQ_ADMIN_PASSWORD@$rmq_hostport/$rmq_path"
export RMQ_ADMIN_BARE_URL="$rmq_proto$rmq_hostport/$rmq_path"
export AMQP_ADMIN=$RMQ_ADMIN_URL
```

You can now use a command such as the following to get a list of queues as a test:

```
curl -s -i -u $RMQ_ADMIN_USER:$RMQ_ADMIN_PASSWORD ${RMQ_ADMIN_URL}api/queues
```

Information concerning the AWS RabbitMQ offering can be found at, https://aws.amazon.com/blogs/aws/amazon-mq-update-new-rabbitmq-message-broker-service/.

In order to use the AWS MQ references in the runners you should set the ConfigMap entries, AMQP_URL, and AMQP_MGT_URL to the values in RMQ_FULL_URL, and RMQ_ADMIN_URL respectively.

### AWS SQS

In order to deploy the runner SQS credentials will need to be injected into the EKS cluster.  A default section must existing within the AWS credentials files, this will be the one selected by the runner. Using the following we can inject all of our known AWS credentials etc into the SQS secrets, this will not always be the best practice and you will need to determine how you will manage these credentials.

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

When the deployment yaml is kubectl applied a set of mount points are included that will map these secrets from the etcd based secrets store for your cluster into the runner containers automatically.

## Deployment of the runner

Having deployed the needed secrets for the choosen queue type the runner can now be deployed.  A template for deployment can be found at examples/aws/deployment.yaml.  The template depends on the environment variables that have been described throughout this document.

```
kubectl apply -f <(stencil -input examples/aws/deployment.yaml)
```

Be aware that any person, or entity having access to the kubernetes vault can extract these secrets unless extra measures are taken to first encrypt the secrets before injecting them into the cluster.
For more information as to how to used secrets hosted through the file system on a running k8s container please refer to, https://kubernetes.io/docs/concepts/configuration/secret/#using-secrets-as-files-from-a-pod.



## Manually accessing cluster master APIs

In order to retrieve the Kubernetes API Bearer token you can use the following command: 

```
kops get secrets --type secret admin -oplaintext
```

Access for the administrative API can be exposed using one of the two following commands:

```
kops get secrets kube -oplaintext
kubectl config view --minify
```

More information concerning the kubelet security can be found at, https://github.com/kubernetes/kops/blob/master/docs/security.md#kubelet-api.

If you wish to pass the ability to manage your cluster to another person, or wish to migrate running the dashboard using a browser on another machine you can using the kops export command to pass a kubectl configuration file around, take care however as this will greatly increase the risk of a security incident if not done correctly.  The configuration for accessing your cluster will be stored in your $KUBECONFIG file, defaulting to $HOME/.kube/config if not defined in your environment table.


If you wish to delete the cluster you can use the following command:

```
$ kops delete cluster $AWS_CLUSTER_NAME --yes
```

Copyright © 2019-2021 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
