# studio-go-runner AWS support

This document details the installation of the studio go runner within an AWS hosted Kubernetes cluster.  After completing the Kubernetes installation using these instructions please return to the main README.md file to continue.

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

For AWS the eksctl tool is now considered the official tool for the EKS CLI.  A full set of instructions for the installation of eksctl can be found at,https://docs.aws.amazon.com/eks/latest/userguide/getting-started-eksctl.html. In brief form eksctl can be installed using the following steps:

```shell
pip install awscli --upgrade --user
curl --silent --location "https://github.com/weaveworks/eksctl/releases/latest/download/eksctl_$(uname -s)_amd64.tar.gz" | tar xz -C /tmp
sudo rm /usr/local/bin/eksctl
sudo mv /tmp/eksctl /usr/local/bin/eksctl
sudo apt-get install jq
```

One requirement of using eksctl is that you must first subscribe to the AMI that will be used with your GPU EC2 instances.  The subscription can be found at, https://aws.amazon.com/marketplace/pp/B07GRHFXGM.


## AWS Cloud support for Kubernetes 1.19.x and GPU

This section discusses the use of eksctl to provision a working k8s cluster onto which the gpu runner can be deployed.

The use of AWS EC2 machines requires that the AWS account has had an EC2 key Pair imported from your administration machine, or created in order that machines created using eksctl can be accessed.  More information can be found at https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-key-pairs.html.

In order to make use of StudioML environment variable based templates you should export the AWS environment variables.  While doing this you should also synchronize your system clock as this is a common source of authentication issues with AWS.  

<pre><code><b>export AWS_ACCOUNT=`aws sts get-caller-identity --query Account --output text`
aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin $AWS_ACCOUNT.dkr.ecr.us-west-2.amazonaws.com
export AWS_ACCESS_KEY=xxx
export AWS_SECRET_ACCESS_KEY=xxx
export AWS_DEFAULT_REGION=xxx
sudo ntpdate ntp.ubuntu.com
export KUBECONFIG=~/.kube/config
export AWS_CLUSTER_NAME=test-eks
</b></code></pre>

The cluster creation options are set using a yaml file, this example uses examples/aws/cluster.yaml which you should modify prior to use:

<pre><code><b>
eksctl create cluster -f <(stencil -input examples/aws/cluster.yaml)
2021-04-01 19:11:08 [ℹ]  eksctl version 0.44.0
2021-04-01 19:11:08 [ℹ]  using region us-west-2
2021-04-01 19:11:08 [ℹ]  subnets for us-west-2a - public:192.168.0.0/19 private:192.168.96.0/19
2021-04-01 19:11:08 [ℹ]  subnets for us-west-2b - public:192.168.32.0/19 private:192.168.128.0/19
2021-04-01 19:11:08 [ℹ]  subnets for us-west-2d - public:192.168.64.0/19 private:192.168.160.0/19
2021-04-01 19:11:09 [ℹ]  nodegroup "overhead" will use "ami-07429ae6ce65be89a" [AmazonLinux2/1.19]
2021-04-01 19:11:09 [ℹ]  using SSH public key "/home/kmutch/.ssh/id_rsa.pub" as "eksctl-test-eks-nodegroup-overhead-be:07:a0:27:44:d8:27:04:c2:ba:28:fa:8c:47:7f:09"
2021-04-01 19:11:09 [ℹ]  nodegroup "1-gpu-spot-p2-xlarge" will use "ami-01f2fad57776fe43f" [AmazonLinux2/1.19]
2021-04-01 19:11:09 [ℹ]  using SSH public key "/home/kmutch/.ssh/id_rsa.pub" as "eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-be:07:a0:27:44:d8:27:04:c2:ba:28:fa:8c:47:7f:09"
2021-04-01 19:11:09 [ℹ]  using Kubernetes version 1.19
2021-04-01 19:11:09 [ℹ]  creating EKS cluster "test-eks" in "us-west-2" region with un-managed nodes
2021-04-01 19:11:09 [ℹ]  2 nodegroups (1-gpu-spot-p2-xlarge, overhead) were included (based on the include/exclude rules)
2021-04-01 19:11:09 [ℹ]  will create a CloudFormation stack for cluster itself and 2 nodegroup stack(s)
2021-04-01 19:11:09 [ℹ]  will create a CloudFormation stack for cluster itself and 0 managed nodegroup stack(s)
2021-04-01 19:11:09 [ℹ]  if you encounter any issues, check CloudFormation console or try 'eksctl utils describe-stacks --region=us-west-2 --cluster=test-eks'
2021-04-01 19:11:09 [ℹ]  Kubernetes API endpoint access will use default of {publicAccess=true, privateAccess=false} for cluster "test-eks" in "us-west-2"
2021-04-01 19:11:09 [ℹ]  2 sequential tasks: { create cluster control plane "test-eks", 3 sequential sub-tasks: { 3 sequential sub-tasks: { wait for control plane to become ready, tag cluster, update CloudWatch logging configuration }, create addons, 2 parallel sub-tasks: { create nodegroup "overhead", create nodegroup "1-gpu-spot-p2-xlarge" } } }
2021-04-01 19:11:09 [ℹ]  building cluster stack "eksctl-test-eks-cluster"
2021-04-01 19:11:10 [ℹ]  deploying stack "eksctl-test-eks-cluster"
2021-04-01 19:11:40 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-cluster"
2021-04-01 19:12:10 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-cluster"
...
2021-04-01 19:23:10 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-cluster"
2021-04-01 19:24:10 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-cluster"
2021-04-01 19:24:11 [✔]  tagged EKS cluster (environment=test-eks)
2021-04-01 19:24:12 [ℹ]  waiting for requested "LoggingUpdate" in cluster "test-eks" to succeed
2021-04-01 19:24:29 [ℹ]  waiting for requested "LoggingUpdate" in cluster "test-eks" to succeed
2021-04-01 19:24:46 [ℹ]  waiting for requested "LoggingUpdate" in cluster "test-eks" to succeed
2021-04-01 19:24:46 [✔]  configured CloudWatch logging for cluster "test-eks" in "us-west-2" (enabled types: audit, authenticator, controllerManager &
disabled types: api, scheduler)
2021-04-01 19:24:46 [ℹ]  building nodegroup stack "eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge"
2021-04-01 19:24:46 [ℹ]  building nodegroup stack "eksctl-test-eks-nodegroup-overhead"
2021-04-01 19:24:47 [ℹ]  deploying stack "eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge"
2021-04-01 19:24:47 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge"
2021-04-01 19:24:47 [ℹ]  deploying stack "eksctl-test-eks-nodegroup-overhead"
2021-04-01 19:24:47 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-nodegroup-overhead"
2021-04-01 19:25:02 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge"
2021-04-01 19:25:07 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-nodegroup-overhead"
2021-04-01 19:25:21 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge"
2021-04-01 19:25:25 [ℹ]  waiting for CloudFormation stack "eksctl-test-eks-nodegroup-overhead"
...
2021-04-01 19:27:55 [ℹ]  waiting for the control plane availability...
2021-04-01 19:27:55 [✔]  saved kubeconfig as "/home/kmutch/.kube/config"
2021-04-01 19:27:55 [ℹ]  as you are using a GPU optimized instance type you will need to install NVIDIA Kubernetes device plugin.
2021-04-01 19:27:55 [ℹ]          see the following page for instructions: https://github.com/NVIDIA/k8s-device-plugin
2021-04-01 19:27:55 [ℹ]  no tasks
2021-04-01 19:27:55 [✔]  all EKS cluster resources for "test-eks" have been created
2021-04-01 19:27:55 [ℹ]  adding identity "arn:aws:iam::613076437200:role/eksctl-test-eks-nodegroup-overhea-NodeInstanceRole-1SJ5R46STPRJK" to auth ConfigMap
2021-04-01 19:27:55 [ℹ]  adding identity "arn:aws:iam::613076437200:role/eksctl-test-eks-nodegroup-1-gpu-s-NodeInstanceRole-12WIJDK3B3AZO" to auth ConfigMap
2021-04-01 19:27:58 [ℹ]  kubectl command should work with "/home/kmutch/.kube/config", try 'kubectl get nodes'
2021-04-01 19:27:58 [✔]  EKS cluster "test-eks" in "us-west-2" region is ready
</code></pre>

eksctl is written in Go uses CloudFormation internally and supports the use of YAML resources to define deployments, more information can be found at https://eksctl.io/.

When creating a cluster the credentials will be loaded into your ~/.kube/config file automatically.  When using the AWS service oriented method of deployment the normally visible master will not be displayed as a node.

The next step is to install the auto scaler that Kubernetes offers.  The auto scaler is installed uisng the following step:

<pre><code><b>
kubectl apply -f examples/aws/autoscaler.yaml</b>
serviceaccount/cluster-autoscaler created
clusterrole.rbac.authorization.k8s.io/cluster-autoscaler created
role.rbac.authorization.k8s.io/cluster-autoscaler created
clusterrolebinding.rbac.authorization.k8s.io/cluster-autoscaler created
rolebinding.rbac.authorization.k8s.io/cluster-autoscaler created
deployment.apps/cluster-autoscaler created
</code></pre>

## GPU Setup

In order to activate GPU support within the workers a daemon set instance needs to be created that will mediate between the kubernetes plugin and the GPU resources available to pods, as shown in the following command.

<pre><code><b>
kubectl apply -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/1.0.0-beta6/nvidia-device-plugin.yml</b>
daemonset.apps/nvidia-device-plugin-daemonset created
</code></pre>

Machines when first started will have an allocatable resource named nvidia.com/gpu.  When this resource flips from 0 to 1 the machine has become available for GPU work.  The plugin yaml added will cause a container to be bootstrapped into new nodes to perform the installation of the drivers etc.

You will be able to run the following command after the Cluster Smoke testing has started to identify the new node added by the auto scaler.

<pre><code><b>
kubectl get nodes "-o=custom-columns=NAME:.metadata.name,GPU:.status.allocatable.nvidia\.com/gpu"</b>
NAME                                         GPU
ip-192-168-5-16.us-west-2.compute.internal   1
</code></pre>

## Cluster Smoke Testing

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
  tolerations:
  # This toleration will allow the gpu hook to run anywhere
  #   By default this is permissive in case you have tainted your GPU nodes.
  - operator: "Exists"
EOF
```

Once the pod has been added the auto scaler log will display output inidiicating that a new node is required to fullfill the work:

```
$ kubectl get pods --namespace kube-system
NAME                                   READY   STATUS    RESTARTS   AGE
aws-node-9rh9k                         1/1     Running   0          3d1h
aws-node-rjdgm                         1/1     Running   0          3d1h
cluster-autoscaler-6446d7bf4f-brvw5    1/1     Running   0          59m
coredns-6548845887-9r4kz               1/1     Running   0          3d1h
coredns-6548845887-fdkd9               1/1     Running   0          3d1h
kube-proxy-ll6jp                       1/1     Running   0          3d1h
kube-proxy-x44pm                       1/1     Running   0          3d1h
nvidia-device-plugin-daemonset-kgskl   1/1     Running   0          58m
nvidia-device-plugin-daemonset-lcdr7   1/1     Running   0          58m
$ kubectl logs --namespace kube-system cluster-autoscaler-6446d7bf4f-brvw5
...
I0405 19:59:41.604787       1 static_autoscaler.go:229] Starting main loop
I0405 19:59:41.605694       1 filter_out_schedulable.go:65] Filtering out schedulables
I0405 19:59:41.605720       1 filter_out_schedulable.go:132] Filtered out 0 pods using hints
I0405 19:59:41.605806       1 filter_out_schedulable.go:170] 0 pods were kept as unschedulable based on caching
I0405 19:59:41.605818       1 filter_out_schedulable.go:171] 0 pods marked as unschedulable can be scheduled.
I0405 19:59:41.605834       1 filter_out_schedulable.go:82] No schedulable pods
I0405 19:59:41.605910       1 klogx.go:86] Pod default/tf-gpu is unschedulable
I0405 19:59:41.605960       1 scale_up.go:364] Upcoming 0 nodes
I0405 19:59:41.606130       1 scale_up.go:288] Pod tf-gpu can't be scheduled on eksctl-test-eks-nodegroup-overhead-NodeGroup-18WE8ZI39VZF7, predicate checking error: Insufficient nvidia.com/gpu; predicateName=NodeResourcesFit; reasons: Insufficient nvidia.com/gpu; debugInfo=
I0405 19:59:41.606157       1 scale_up.go:437] No pod can fit to eksctl-test-eks-nodegroup-overhead-NodeGroup-18WE8ZI39VZF7
I0405 19:59:41.606171       1 waste.go:57] Expanding Node Group eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-165ZZ5GD15VO2 would waste 100.00% CPU, 98.36% Memory, 99.18% Blended
I0405 19:59:41.606205       1 scale_up.go:456] Best option to resize: eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-165ZZ5GD15VO2
I0405 19:59:41.606220       1 scale_up.go:460] Estimated 1 nodes needed in eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-165ZZ5GD15VO2
I0405 19:59:41.606258       1 scale_up.go:574] Final scale-up plan: [{eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-165ZZ5GD15VO2 0->1 (max: 10)}]
I0405 19:59:41.606287       1 scale_up.go:663] Scale-up: setting group eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-165ZZ5GD15VO2 size to 1
I0405 19:59:41.606373       1 auto_scaling_groups.go:219] Setting asg eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-165ZZ5GD15VO2 size to 1
I0405 19:59:41.606673       1 event_sink_logging_wrapper.go:48] Event(v1.ObjectReference{Kind:"ConfigMap", Namespace:"kube-system", Name:"cluster-autoscaler-status", UID:"e3eb5ed4-7962-4017-94d2-dc5d71963440", APIVersion:"v1", ResourceVersion:"736946", FieldPath:""}): type: 'Normal' reason: 'ScaledUpGroup' Scale-up: setting group eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-165ZZ5GD15VO2 size to 1
I0405 19:59:41.757570       1 eventing_scale_up_processor.go:47] Skipping event processing for unschedulable pods since there is a ScaleUp attempt this loop
I0405 19:59:41.758074       1 event_sink_logging_wrapper.go:48] Event(v1.ObjectReference{Kind:"Pod", Namespace:"default", Name:"tf-gpu", UID:"ab39c253-cd0b-4670-8ee5-3122e8ad6db1", APIVersion:"v1", ResourceVersion:"736857", FieldPath:""}): type: 'Normal' reason: 'TriggeredScaleUp' pod triggered scale-up: [{eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-165ZZ5GD15VO2 0->1 (max: 10)}]
...
```

The new node is added resulting in

```
$ kubectl get nodes
NAME                                           STATUS   ROLES    AGE     VERSION
ip-192-168-27-155.us-west-2.compute.internal   Ready    <none>   2m16s   v1.19.6-eks-49a6c0
ip-192-168-3-184.us-west-2.compute.internal    Ready    <none>   3d1h    v1.19.6-eks-49a6c0
ip-192-168-4-192.us-west-2.compute.internal    Ready    <none>   3d1h    v1.19.6-eks-49a6c0
$ kubectl get pods
NAME     READY   STATUS              RESTARTS   AGE
tf-gpu   0/1     ContainerCreating   0          5m47s
```


If the new node does not appear, and the auto scaler log shows the tf-gpu pod is to be scheduled on the cloudformation template results there can be a number of causes.  The message that indicates cloud formation has been invoked to add the node will appear as follows:

```
I0409 13:57:25.371521       1 filter_out_schedulable.go:157] Pod default.tf-gpu marked as unschedulable can be scheduled on node template-node for-eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-114XB1S03EMHG-8505906760983331750-0. Ignoring in scale up.
```
Scaling activities can be obtained using the following commands to assist in diagnosing what is occuring within the scaler:

<pre><code><b>
aws autoscaling describe-auto-scaling-groups | jq -r '..|.AutoScalingGroupName?' |grep eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge</b>
eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-HKH7E4GCQ3GP
<b>aws autoscaling describe-scaling-activities --auto-scaling-group-name eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-HKH7E4GCQ3GP | jq '.Activities[0]'</b>
{
  "ActivityId": "96f5e2df-604d-9ad0-1598-2672c388e498",
  "AutoScalingGroupName": "eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-HKH7E4GCQ3GP",
  "Description": "Launching a new EC2 instance.  Status Reason: Could not launch Spot Instances. UnfulfillableCapacity - There is no capacity availabl
e that matches your request. Launching EC2 instance failed.",
  "Cause": "At 2021-04-09T18:14:53Z an instance was started in response to a difference between desired and actual capacity, increasing the capacity f
rom 0 to 1.",
  "StartTime": "2021-04-09T18:14:54.566Z",
  "EndTime": "2021-04-09T18:14:54Z",
  "StatusCode": "Failed",
  "StatusMessage": "Could not launch Spot Instances. UnfulfillableCapacity - There is no capacity available that matches your request. Launching EC2 i
nstance failed.",
  "Progress": 100,
  "Details": "{\"Subnet ID\":\"subnet-0853b684808f1ad07\",\"Availability Zone\":\"us-west-2a\"}",
  "AutoScalingGroupARN": "arn:aws:autoscaling:us-west-2:...:autoScalingGroup:74dd6499-6426-488f-98eb-35e5bea961cc:autoScalingGroupName/eksctl
-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-HKH7E4GCQ3GP"
}
```

The jq command was used to select the first, or latest scaling acitivity.  The failed scaling attempt was due to the availability zones specified having no capacity.  TGhe fix would be to modify the node group definition inside the cluster.yaml file and redeploy the cluster in zones that have availability of the instance types being used.

Once the pod is in a running state you should be able to test the access to the GPU cards using the following commands:

<pre><code><b>
kubectl get pods</b>
NAME     READY   STATUS    RESTARTS   AGE
tf-gpu   1/1     Running   0          2m31s
 <b>kubectl exec -it tf-gpu -- \
  python -c 'from tensorflow.python.client import device_lib; print(device_lib.list_local_devices())'</b>
2021-04-05 20:09:20.487509: W tensorflow/core/profiler/internal/smprofiler_timeline.cc:460] Initializing the SageMaker Profiler.
2021-04-05 20:09:20.487672: W tensorflow/core/profiler/internal/smprofiler_timeline.cc:105] SageMaker Profiler is not enabled. The timeline writer thread will not be started, future recorded events will be dropped.
2021-04-05 20:09:20.494959: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcudart.so.11.0
2021-04-05 20:09:20.530896: W tensorflow/core/profiler/internal/smprofiler_timeline.cc:460] Initializing the SageMaker Profiler.
2021-04-05 20:09:22.160495: I tensorflow/core/platform/profile_utils/cpu_utils.cc:104] CPU Frequency: 2300010000 Hz
2021-04-05 20:09:22.160965: I tensorflow/compiler/xla/service/service.cc:168] XLA service 0x557b6ed99900 initialized for platform Host (this does not guarantee that XLA will be used). Devices:
2021-04-05 20:09:22.161038: I tensorflow/compiler/xla/service/service.cc:176]   StreamExecutor device (0): Host, Default Version
2021-04-05 20:09:22.164066: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcuda.so.1
2021-04-05 20:09:22.310055: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:982] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2021-04-05 20:09:22.311028: I tensorflow/compiler/xla/service/service.cc:168] XLA service 0x557b6ee20470 initialized for platform CUDA (this does not guarantee that XLA will be used). Devices:
2021-04-05 20:09:22.311068: I tensorflow/compiler/xla/service/service.cc:176]   StreamExecutor device (0): Tesla K80, Compute Capability 3.7
2021-04-05 20:09:22.311348: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:982] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2021-04-05 20:09:22.312171: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1716] Found device 0 with properties:
pciBusID: 0000:00:1e.0 name: Tesla K80 computeCapability: 3.7
coreClock: 0.8235GHz coreCount: 13 deviceMemorySize: 11.17GiB deviceMemoryBandwidth: 223.96GiB/s
2021-04-05 20:09:22.312236: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcudart.so.11.0
2021-04-05 20:09:22.315893: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcublas.so.11
2021-04-05 20:09:22.317468: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcufft.so.10
2021-04-05 20:09:22.317876: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcurand.so.10
2021-04-05 20:09:22.321294: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcusolver.so.10
2021-04-05 20:09:22.322155: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcusparse.so.11
2021-04-05 20:09:22.322412: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcudnn.so.8
2021-04-05 20:09:22.322565: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:982] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2021-04-05 20:09:22.323432: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:982] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2021-04-05 20:09:22.324228: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1858] Adding visible gpu devices: 0
2021-04-05 20:09:22.324287: I tensorflow/stream_executor/platform/default/dso_loader.cc:48] Successfully opened dynamic library libcudart.so.11.0
2021-04-05 20:09:22.772479: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1257] Device interconnect StreamExecutor with strength 1 edge matrix:
2021-04-05 20:09:22.772539: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1263]      0
2021-04-05 20:09:22.772563: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1276] 0:   N
2021-04-05 20:09:22.772849: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:982] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2021-04-05 20:09:22.773761: I tensorflow/stream_executor/cuda/cuda_gpu_executor.cc:982] successful NUMA node read from SysFS had negative value (-1), but there must be at least one NUMA node, so returning NUMA node zero
2021-04-05 20:09:22.774565: I tensorflow/core/common_runtime/gpu/gpu_device.cc:1402] Created TensorFlow device (/device:GPU:0 with 10623 MB memory) -> physical GPU (device: 0, name: Tesla K80, pci bus id: 0000:00:1e.0, compute capability: 3.7)
[name: "/device:CPU:0"
device_type: "CPU"
memory_limit: 268435456
locality {
}
incarnation: 10414284085485766931
, name: "/device:XLA_CPU:0"
device_type: "XLA_CPU"
memory_limit: 17179869184
locality {
}
incarnation: 12659882986103904376
physical_device_desc: "device: XLA_CPU device"
, name: "/device:XLA_GPU:0"
device_type: "XLA_GPU"
memory_limit: 17179869184
locality {
}
incarnation: 4671966972074686993
physical_device_desc: "device: XLA_GPU device"
, name: "/device:GPU:0"
device_type: "GPU"
memory_limit: 11139760768
locality {
  bus_id: 1
  links {
  }
}
incarnation: 4261672894508981255
physical_device_desc: "device: 0, name: Tesla K80, pci bus id: 0000:00:1e.0, compute capability: 3.7"
]
<b>kubectl exec -it tf-gpu -- nvidia-smi</b>
Mon Apr  5 20:08:27 2021
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 460.32.03    Driver Version: 460.32.03    CUDA Version: 11.2     |
|-------------------------------+----------------------+----------------------+
| GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
| Fan  Temp  Perf  Pwr:Usage/Cap|         Memory-Usage | GPU-Util  Compute M. |
|                               |                      |               MIG M. |
|===============================+======================+======================|
|   0  Tesla K80           Off  | 00000000:00:1E.0 Off |                    0 |
| N/A   31C    P8    32W / 149W |      0MiB / 11441MiB |      0%      Default |
|                               |                      |                  N/A |
+-------------------------------+----------------------+----------------------+

+-----------------------------------------------------------------------------+
| Processes:                                                                  |
|  GPU   GI   CI        PID   Type   Process name                  GPU Memory |
|        ID   ID                                                   Usage      |
|=============================================================================|
|  No running processes found                                                 |
+-----------------------------------------------------------------------------+
<b>kubectl delete pod tf-gpu</b>
pod "tf-gpu" deleted
</code></pre>

Once the pod is deleted the auto scaler will begin to scale down, node scaling events happen after 10 minutes of inactivity on the nodes:

```
I0405 20:21:48.936908       1 static_autoscaler.go:229] Starting main loop
I0405 20:21:48.937342       1 taints.go:77] Removing autoscaler soft taint when creating template from node
I0405 20:21:48.937626       1 filter_out_schedulable.go:65] Filtering out schedulables
I0405 20:21:48.937649       1 filter_out_schedulable.go:132] Filtered out 0 pods using hints
I0405 20:21:48.937657       1 filter_out_schedulable.go:170] 0 pods were kept as unschedulable based on caching
I0405 20:21:48.937664       1 filter_out_schedulable.go:171] 0 pods marked as unschedulable can be scheduled.
I0405 20:21:48.937679       1 filter_out_schedulable.go:82] No schedulable pods
I0405 20:21:48.937710       1 static_autoscaler.go:402] No unschedulable pods
I0405 20:21:48.937731       1 static_autoscaler.go:449] Calculating unneeded nodes
I0405 20:21:48.937782       1 scale_down.go:421] Node ip-192-168-27-155.us-west-2.compute.internal - nvidia.com/gpu utilization 0.000000
I0405 20:21:48.937821       1 scale_down.go:487] Scale-down calculation: ignoring 2 nodes unremovable in the last 5m0s
I0405 20:21:48.937924       1 static_autoscaler.go:492] ip-192-168-27-155.us-west-2.compute.internal is unneeded since 2021-04-05 20:11:45.759037653 +0000 UTC m=+4195.405515037 duration 10m3.177802803s
I0405 20:21:48.937958       1 static_autoscaler.go:503] Scale down status: unneededOnly=false lastScaleUpTime=2021-04-05 19:59:41.604745396 +0000 UTC m=+3471.251222503 lastScaleDownDeleteTime=2021-04-05 19:02:12.392306441 +0000 UTC m=+22.038783488 lastScaleDownFailTime=2021-04-05 19:02:12.392308118 +0000 UTC m=+22.038785370 scaleDownForbidden=false isDeleteInProgress=false scaleDownInCooldown=false
I0405 20:21:48.937980       1 static_autoscaler.go:516] Starting scale down
I0405 20:21:48.938035       1 scale_down.go:790] ip-192-168-27-155.us-west-2.compute.internal was unneeded for 10m3.177802803s
I0405 20:21:48.938072       1 scale_down.go:1053] Scale-down: removing empty node ip-192-168-27-155.us-west-2.compute.internal
I0405 20:21:48.938274       1 event_sink_logging_wrapper.go:48] Event(v1.ObjectReference{Kind:"ConfigMap", Namespace:"kube-system", Name:"cluster-autoscaler-status", UID:"e3eb5ed4-7962-4017-94d2-dc5d71963440", APIVersion:"v1", ResourceVersion:"741638", FieldPath:""}): type: 'Normal' reason: 'ScaleDownEmpty' Scale-down: removing empty node ip-192-168-27-155.us-west-2.compute.internal
I0405 20:21:48.951429       1 delete.go:103] Successfully added ToBeDeletedTaint on node ip-192-168-27-155.us-west-2.compute.internal
I0405 20:21:49.206708       1 auto_scaling_groups.go:277] Terminating EC2 instance: i-01da70a9349280c94
I0405 20:21:49.206738       1 aws_manager.go:297] Some ASG instances might have been deleted, forcing ASG list refresh
I0405 20:21:49.283533       1 auto_scaling_groups.go:351] Regenerating instance to ASG map for ASGs: [eksctl-test-eks-nodegroup-1-gpu-spot-p2-xlarge-NodeGroup-165ZZ5
GD15VO2 eksctl-test-eks-nodegroup-overhead-NodeGroup-18WE8ZI39VZF7]
I0405 20:21:49.397556       1 auto_scaling.go:199] 2 launch configurations already in cache
I0405 20:21:49.397810       1 aws_manager.go:269] Refreshed ASG list, next refresh after 2021-04-05 20:22:49.397803109 +0000 UTC m=+4859.044280298
I0405 20:21:49.397981       1 event_sink_logging_wrapper.go:48] Event(v1.ObjectReference{Kind:"Node", Namespace:"", Name:"ip-192-168-27-155.us-west-2.compute.interna
l", UID:"14287283-b1d8-4c7f-8b3f-2d0d66581467", APIVersion:"v1", ResourceVersion:"741465", FieldPath:""}): type: 'Normal' reason: 'ScaleDown' node removed by cluster autoscaler
```

After this the node will be marked as NotReady and shortly after the node will disappear:

```
$ kubectl get nodes
NAME                                           STATUS     ROLES    AGE    VERSION
ip-192-168-27-155.us-west-2.compute.internal   NotReady   <none>   22m    v1.19.6-eks-49a6c0
ip-192-168-3-184.us-west-2.compute.internal    Ready      <none>   3d1h   v1.19.6-eks-49a6c0
ip-192-168-4-192.us-west-2.compute.internal    Ready      <none>   3d1h   v1.19.6-eks-49a6c0
$ kubectl get nodes
NAME                                          STATUS   ROLES    AGE    VERSION
ip-192-168-3-184.us-west-2.compute.internal   Ready    <none>   3d1h   v1.19.6-eks-49a6c0
ip-192-168-4-192.us-west-2.compute.internal   Ready    <none>   3d1h   v1.19.6-eks-49a6c0
```

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
