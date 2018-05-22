# studio-go-runner AWS support

### Install kops (AWS only)

If you are using azure or GCP then options such as acs-engine, and skaffold are natively supported by the cloud vendors and written in Go so are readily usable and can be easily customized and maintained and so these are recommended for those cases.

<pre><code><b>curl -LO https://github.com/kubernetes/kops/releases/download/1.9.0/kops-linux-amd64
chmod +x kops-linux-amd64
sudo mv kops-linux-amd64 /usr/local/bin/kops

Add kubectl autocompletion to your current shell:

source <(kops completion bash)
</b></code></pre>

## AWS Cloud support for Kubernetes

This section discusses the use of kops to provision a working k8s cluster onto which the runner can be deployed.

kops makes use of an S3 bucket to store cluster configurations.

In order to seed your S3 KOPS_STATE_STORE version controlled bucket with a cluster definition the following command could be used:

<pre><code><b>export AWS_AVAILABILITY_ZONES="$(aws ec2 describe-availability-zones --query 'AvailabilityZones[].ZoneName' --output text | awk -v OFS="," '$1=$1')"

export S3_BUCKET=kops-platform-$USER
export KOPS_STATE_STORE=s3://$S3_BUCKET
aws s3 mb $KOPS_STATE_STORE
aws s3api put-bucket-versioning --bucket $S3_BUCKET --versioning-configuration Status=Enabled

export CLUSTER_NAME=test-$USER.platform.cluster.k8s.local

kops create cluster --name $CLUSTER_NAME --zones $AWS_AVAILABILITY_ZONES --node-count 1
</b></code></pre>

Optionally use an image from your preferred zone e.g. --image=ami-0def3275.  Also you can modify the AWS machine types, recommended during developer testing using options such as '--master-size=m4.large --node-size=m4.large'.

Starting the cluster can now be done using the following command:

<pre><code><b>kops update cluster $CLUSTER_NAME --yes</b>
I0309 13:48:49.798777    6195 apply_cluster.go:442] Gossip DNS: skipping DNS validation
I0309 13:48:49.961602    6195 executor.go:91] Tasks: 0 done / 81 total; 30 can run
I0309 13:48:50.383671    6195 vfs_castore.go:715] Issuing new certificate: "ca"
I0309 13:48:50.478788    6195 vfs_castore.go:715] Issuing new certificate: "apiserver-aggregator-ca"
I0309 13:48:50.599605    6195 executor.go:91] Tasks: 30 done / 81 total; 26 can run
I0309 13:48:51.013957    6195 vfs_castore.go:715] Issuing new certificate: "kube-controller-manager"
I0309 13:48:51.087447    6195 vfs_castore.go:715] Issuing new certificate: "kube-proxy"
I0309 13:48:51.092714    6195 vfs_castore.go:715] Issuing new certificate: "kubelet"
I0309 13:48:51.118145    6195 vfs_castore.go:715] Issuing new certificate: "apiserver-aggregator"
I0309 13:48:51.133527    6195 vfs_castore.go:715] Issuing new certificate: "kube-scheduler"
I0309 13:48:51.157876    6195 vfs_castore.go:715] Issuing new certificate: "kops"
I0309 13:48:51.167195    6195 vfs_castore.go:715] Issuing new certificate: "apiserver-proxy-client"
I0309 13:48:51.172542    6195 vfs_castore.go:715] Issuing new certificate: "kubecfg"
I0309 13:48:51.179730    6195 vfs_castore.go:715] Issuing new certificate: "kubelet-api"
I0309 13:48:51.431304    6195 executor.go:91] Tasks: 56 done / 81 total; 21 can run
I0309 13:48:51.568136    6195 launchconfiguration.go:334] waiting for IAM instance profile "nodes.test.platform.cluster.k8s.local" to be ready
I0309 13:48:51.576067    6195 launchconfiguration.go:334] waiting for IAM instance profile "masters.test.platform.cluster.k8s.local" to be ready
I0309 13:49:01.973887    6195 executor.go:91] Tasks: 77 done / 81 total; 3 can run
I0309 13:49:02.489343    6195 vfs_castore.go:715] Issuing new certificate: "master"
I0309 13:49:02.775403    6195 executor.go:91] Tasks: 80 done / 81 total; 1 can run
I0309 13:49:03.074583    6195 executor.go:91] Tasks: 81 done / 81 total; 0 can run
I0309 13:49:03.168822    6195 update_cluster.go:279] Exporting kubecfg for cluster
kops has set your kubectl context to test.platform.cluster.k8s.local

Cluster is starting.  It should be ready in a few minutes.

Suggestions:
 * validate cluster: kops validate cluster
 * list nodes: kubectl get nodes --show-labels
 * ssh to the master: ssh -i ~/.ssh/id_rsa admin@api.test.platform.cluster.k8s.local
 * the admin user is specific to Debian. If not using Debian please use the appropriate user based on your OS.
 * read about installing addons at: https://github.com/kubernetes/kops/blob/master/docs/addons.md.

</code></pre>

The initial cluster spinup will take sometime, use kops commands such as 'kops validate cluster' to determine when the cluster is spun up ready for the runner to be deployed as a k8s container.

If you wish to delete the cluster you can use the following command:

```
$ az group delete --name $k8s_resource_group --yes --no-wait
```
