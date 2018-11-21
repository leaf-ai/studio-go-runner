# studio-go-runner AWS support

### Install kops (AWS only)

If you are using azure or GCP then options such as acs-engine, and skaffold are natively supported by the cloud vendors.  These tools are also readily customizable, and maintained and so these are recommended.

For AWS the kops tool is consider the best practice currently and can be installed using the following steps.

<pre><code><b>curl -LO https://github.com/kubernetes/kops/releases/download/1.10.0/kops-linux-amd64
chmod +x kops-linux-amd64
sudo mv kops-linux-amd64 /usr/local/bin/kops

Add kubectl autocompletion to your current shell:

source <(kops completion bash)
</b></code></pre>

## AWS Cloud support for Kubernetes 1.11.x and GPU (Prototyping stage)

This is a work in progress and is on hold until kops can officially support the new k8s plugin driver features.

This section discusses the use of kops to provision a working k8s cluster onto which the gpu runner can be deployed.

kops makes use of an S3 bucket to store cluster configurations.

In order to seed your S3 KOPS_STATE_STORE version controlled bucket with a cluster definition the following command could be used:

<pre><code><b>export AWS_AVAILABILITY_ZONES="$(aws ec2 describe-availability-zones --query 'AvailabilityZones[].ZoneName' --output text | awk -v OFS="," '$1=$1')"

export S3_BUCKET=kops-platform-$USER
export KOPS_STATE_STORE=s3://$S3_BUCKET
aws s3 mb $KOPS_STATE_STORE
aws s3api put-bucket-versioning --bucket $S3_BUCKET --versioning-configuration Status=Enabled

export AWS_CLUSTER_NAME=test-$USER.platform.cluster.k8s.local

kops create cluster --name $AWS_CLUSTER_NAME --zones $AWS_AVAILABILITY_ZONES --node-count 1 --node-size p2.xlarge --ssh-public-key --image kope.io/k8s-1.10-debian-stretch-amd64-hvm-ebs-2018-05-27 --kubernetes-version 1.11.2
</b></code></pre>

You can modify the AWS machine types, recommended during developer testing using options such as '--master-size=m4.large --node-size=m4.large'.

You should now follow instructions related to enabling GPU integration from AWS into Kubernetes as described at https://github.com/dcwangmit01/kops/tree/gpu-device-plugins-3/hooks/nvidia-device-plugin.  In summary the commands to do this are as follows:

<pre><code><b>
kops edit ig --name=$AWS_CLUSTER_NAME nodes
</b></code></pre>

Adding the following yaml lines at the very top of the spec section.

<pre><code><b>
spec:
...
  hooks:
  - execContainer:
      image: dcwangmit01/nvidia-device-plugin:0.1.0
</b></code></pre>

Starting the cluster can now be done using the following command:

<pre><code><b>kops update cluster $AWS_CLUSTER_NAME --yes</b>
I1120 10:21:49.355818   28638 apply_cluster.go:505] Gossip DNS: skipping DNS validation
I1120 10:21:49.658673   28638 executor.go:103] Tasks: 0 done / 81 total; 30 can run
I1120 10:21:50.128780   28638 vfs_castore.go:735] Issuing new certificate: "apiserver-aggregator-ca"
I1120 10:21:50.233444   28638 vfs_castore.go:735] Issuing new certificate: "ca"
I1120 10:21:50.464817   28638 executor.go:103] Tasks: 30 done / 81 total; 26 can run
I1120 10:21:51.069328   28638 vfs_castore.go:735] Issuing new certificate: "kube-controller-manager"
I1120 10:21:51.139103   28638 vfs_castore.go:735] Issuing new certificate: "apiserver-aggregator"
I1120 10:21:51.144279   28638 vfs_castore.go:735] Issuing new certificate: "kops"
I1120 10:21:51.212479   28638 vfs_castore.go:735] Issuing new certificate: "kube-scheduler"
I1120 10:21:51.220052   28638 vfs_castore.go:735] Issuing new certificate: "kubelet"
I1120 10:21:51.284291   28638 vfs_castore.go:735] Issuing new certificate: "kubelet-api"
I1120 10:21:51.369020   28638 vfs_castore.go:735] Issuing new certificate: "apiserver-proxy-client"
I1120 10:21:51.443218   28638 vfs_castore.go:735] Issuing new certificate: "kube-proxy"
I1120 10:21:51.992354   28638 vfs_castore.go:735] Issuing new certificate: "kubecfg"
I1120 10:21:52.556311   28638 executor.go:103] Tasks: 56 done / 81 total; 21 can run
I1120 10:21:53.391206   28638 launchconfiguration.go:380] waiting for IAM instance profile "nodes.test-kmutch.platform.cluster.k8s.local" to be ready
I1120 10:21:53.472543   28638 launchconfiguration.go:380] waiting for IAM instance profile "masters.test-kmutch.platform.cluster.k8s.local" to be ready
I1120 10:22:04.802014   28638 executor.go:103] Tasks: 77 done / 81 total; 3 can run
I1120 10:22:05.741373   28638 vfs_castore.go:735] Issuing new certificate: "master"
I1120 10:22:06.563138   28638 executor.go:103] Tasks: 80 done / 81 total; 1 can run
I1120 10:22:07.509638   28638 executor.go:103] Tasks: 81 done / 81 total; 0 can run
I1120 10:22:07.581306   28638 update_cluster.go:290] Exporting kubecfg for cluster
kops has set your kubectl context to test-kmutch.platform.cluster.k8s.local

Cluster is starting.  It should be ready in a few minutes.

Suggestions:
 * validate cluster: kops validate cluster
 * list nodes: kubectl get nodes --show-labels
 * ssh to the master: ssh -i ~/.ssh/id_rsa admin@api.test-kmutch.platform.cluster.k8s.local
 * the admin user is specific to Debian. If not using Debian please use the appropriate user based on your OS.
 * read about installing addons at: https://github.com/kubernetes/kops/blob/master/docs/addons.md.

</code></pre>

The initial cluster spinup will take sometime, use kops commands such as 'kops validate cluster' to determine when the cluster is spun up ready for the runner to be deployed as a k8s container.

In order to activate GPU support within the workers a daemon set instance needs to be created that will mediate between the kubernetes plugin and the GPU resources available to pods, as shown in the following command.

<pre><code><b>
kubectl create -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v1.11/nvidia-device-plugin.yml
</b></code></pre>

Machines when first started will have an allocatable resource named nvidia.com/gpu.  When this resource flips from 0 to 1 the machine has become available for GPU work.  The hook yaml section added ealier will cause a container to be bootstrapped into new nodes to perform the installation of the drivers etc.

If you wish to delete the cluster you can use the following command:

```
$ kops delete cluster $AWS_CLUSTER_NAME --yes
```
