# Production TensorFlow Model Serving

This application note addresses a common question about which model serving offering should be used once a model is trained using StudioML.  This issue is a common one that arises for both open source machine learning implementations and Cognizant LEAF implementations.

There are a large variety of ways to serve models each with pros and cons.  This document attempts to assist those executing on production ML model seving projects with a simple approach based upon simple conventions and using TensorFlow eXtensions (TFX).

This document does not seek to address questions concerning model management.  Model management is largely a function of the style of machine learning being undertaken and the level to which a choosen learning strategy requires manual intervention.  Traditional single objective approaches requirements for model management can be very different than for those using evolutionary multiobjective learning.  Because of this variability it is not in-scope for this document but it should be noted that model selection and monitoring addresed by the use of evolutionary multi-object learning an approach should generally seek low friction management that minimizes manual gatekeeping workflows typical of traditional approaches.

<!--ts-->

Table of Contents
=================

* [Production TensorFlow Model Serving](#production-tensorflow-model-serving)
* [Table of Contents](#table-of-contents)
  * [Introduction](#introduction)
  * [Google TensorFlow Serving platform](#google-tensorflow-serving-platform)
  * [TensorFlow Serving Simple Workflow](#tensorflow-serving-simple-workflow)
    * [TFX Model Export](#tfx-model-export)
    * [Model Serving Bridge](#model-serving-bridge)
    * [TFXModel Serving configuration](#tfxmodel-serving-configuration)
    * [Export to Serving Bridge](#export-to-serving-bridge)
<!--te-->

## Introduction

TensorFlow model serving options have changed significantly over time.  You should expect that the choosen approach we currently use will change at some point in time, possibly quite quickly so this document should function as a starting point.

This document details how models being uploaded to S3 can be pre-processed by a bridge, which then generates a TFX serving configuration file that is provisioned using a Kubernetes Config Map that is mounted into a TFX model serving Kubernetes pod and is used as its configuration.

## Google TensorFlow Serving platform

TensorFlow model serving is available as a part of the Google production scale machine learning platform known as TFX.  Adopting the serving functionality does not require the wholesale adoption of the Google platform.  StudioML is an automatted Evolutionary ML service for model creation that is an automatted alternative to data scientist orchestrated ML workflows such as KubeFlow.  In both cases training steps result in model files that are stored at a well known location and can be subsequently discovered by the model serving and in turn then load them automatically.

This document discusses using model serving within a Kubernetes context, however the TFX based model serving solution is designed to run in both standalone workstation, Docker, and Kubernetes deployments.  The solutions can also be deployed without being coupled to components such as a database or a specific MLops framework.  In addition TFX model serving also offers opportunities for serving other types of embeddings, non TFX models, and data.  In the case of non TensorFlow models custom software needs to be implemeted which is not discussed in this document, this requires C++ chops and is documented here, [Creating a new kind of servable](https://www.tensorflow.org/tfx/serving/custom_servable).

Packaging for the model serving component is container based and offers both Docker Desktop and Kubernetes deployments.  The configuration options for serving allow local and remote storage locations to be automatically served.

For reference puposes the Kubernetes TFX model serving capability is documented at, https://www.tensorflow.org/tfx/serving/serving\_kubernetes.

Should you wish to make use of the Google gRPC model serving options which require some additional effort and knowledge of the gRPC solutions stack information about this can be obtained from, https://www.tensorflow.org/tfx/serving/serving\_basic.

One of the advanatages of using the model serving bridge is that the use of S3 and the Kubernetes ConfigMap as out integration points can be secured more readily and are generally available to platforms not support gRPC such as JavaScript.

## TensorFlow Serving Simple Workflow

TensorFlow Serving consists of two components, export and serving.  A third component exists that provides a simple bridge between exporting and serving when Kubernetes based deployments are used.

### TFX Model Export

The first step is to train the model and export it using the [TensorFlow SavedModel format](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/python/saved_model/README.md).  Upon export the model and additional assets are saved into a [directory heirarchy](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/python/saved_model/README.md#components) with the top level directory being the version number of the model.

The model export action is typically performed once the evolutionary training has completed, or converged

Once the experiment is complete and ready for deployment the experiment uses python code to invoke the TensorFlow APIs for performing a model export, [Train and export TensorFlow model](https://www.tensorflow.org/tfx/serving/serving_basic#train_and_export_tensorflow_model). The objective of using model export is that we wish the model to be formatting using a broken out directory tree and protobuf files.  If you are using TensorFlow 2.x you will possibly have noticed that this format is now the default for save model operations and the export operation might not be needed at all.

Once exported the experimenter implementing the python orchestration for their experiment then copies the exported directory tree for the model to S3.  In the case where the experimenter has control over the python code used to orchestrate and export the model then boto3, or any other S3 API, can be used to load the model files as blobs to S3.  Alternatively the model can be copied up using an S3 aware file copy tool such as the [Minio client](https://docs.min.io/docs/minio-client-quickstart-guide.html), or the [AWS CLI S3 reference](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/s3/index.html).

If model promotion is not automatted, on inspection of fitness metrics about the multi-objective trade-offs made, experimenters can use a free standing python program to invoke the TensorFlow APIs to perform the export as well.

When the recursive copy of the models components are complete the exporter then writes a CSV index file into the top level directory of the bucket that is named using a UUID, of the callers choice, with a prefix of 'index-'.  The contents of the index file contains be the list of individual files/keys that must be present and observable to the serving before loading the experiments.  Each line in the CSV should have 3 fields, first is the base path for the model, second the fully qualified key of the blobs, or file name, and the third field should be the etag (S3 internally defined checksum) of the blob or file checksum.

### Model Serving Bridge

The next step in the pipeline uses an open source component that can be found in the go runner repository at, https://github.com/leaf-ai/studio-go-runner/tree/main/tools/serving-bridge.

This component can be deployed using a Kubernetes deployment resource.  There is a single deployment dependency for the serving bridge and that is the repsence of an S3 blob store to hold models, for cases involving testing and where a local S3 store is required for on-premises installation minio, https://min.io, can be used.

An example of a minio deployment can be found in the minio.yaml file, this example creates a persistent volume for the minio server with 10Gb of storage.

The main deployment file can be found in deployment.yaml.  The deployment file makes use of the Kubernetes Kustomize tool for injection of secrets, namespaces and other attributes for inclusion into the deployment configuration.  If you are running the bridge in a production environment you should seek to secure secrets using the norms of your enterprise infrastructure.

Kustomize is the Kubernetes project YAML template tooling.  The Kustomize tooling can be installed into your local working directory using the following command:

```
curl -s "https://raw.githubusercontent.com/\
kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"  | bash
```

More information can be found at https://kustomize.io/.  After installation of the kustomize tool the kustomization.yaml file should be created and your secrets for accessing the S3 storage platform should be placed into this file.  An example kustomization.yaml file is provided in the tools/serving-bridge directory of the repository where this README is hosted.

After modifing the kustomization.yaml file, deploy using the following command:

```
kubectl apply -f <(kustomize build)
```

#### Example configuration access

Once the bridge has been deployed you will be able to interact with the minio server deployed with it in test mode.  To do this you should first install the minio client, please refer to instructions at, https://docs.min.io/docs/minio-client-quickstart-guide.html.

Having done this locate the appropriate host and port number for the minio service, for example:

```
$ kubectl --namespace serving-bridge get services -o=wide

NAME            TYPE           CLUSTER-IP       EXTERNAL-IP   PORT(S)          AGE   SELECTOR
minio-service   LoadBalancer   10.152.183.180   <pending>     9000:32372/TCP   15m   app=minio
```


In the above case we are running the kubnernetes cluster using our local host and so the address of the minio server will be 127.0.0.1:32372.  If an external host is being used then the EXTERNAL-IP field can be used to select the appropriate IP address.

The next step is to add the minio host to the minio clients configuration using the following command as an example:

```
$ mc alias set serving http://127.0.0.1:32372 UserUser PasswordPassword
Added `serving` successfully.
```

The default Kustomization.yaml file contains a bucket name that we know create on the server:

```
$ mc mb serving/test-bucket
Bucket created successfully `serving/test-bucket`.
```

The bucket can now be populated with an example model found within this repository at tools/serving-bridge/model_gen/model:

```
$ mc mirror tools/serving-bridge/model_gen serving/test-bucket/example_model/
...les.data-00000-of-00001:  473.09 KiB / 473.09 KiB ┃▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓┃ 20.60 MiB/s 0s
```

Now that a complete bucket has been uploaded the index file needs to be created that matches the model.  Using standard json and text processing tools this can be done once the blobs have been created on your object store, for example:

```
$ export index_uuid=`uuidgen`
$ mc ls serving/test-bucket/example-model --recursive --json | jq -r '.key, .etag' | paste -d ",\n"  - - | awk '{ print "example-model,example-model/" $0; }' > index-${index_uuid}.csv
$ cat index-${index_uuid}.csv
example-model,example-model/1/saved_model.pb,d941f687c5968bc8d1d1cb878371513f
example-model,example-model/1/variables/variables.data-00000-of-00001,44025f1e00607035fe27a28f92fb1ac9
example-model,example-model/1/variables/variables.index,0abf72f415629ad93f62c769ea97f3e6
example-model,example-model/2/saved_model.pb,afd407852fbd7a59ef2c41ebcc8d12ad
example-model,example-model/2/variables/variables.data-00000-of-00001,44025f1e00607035fe27a28f92fb1ac9
example-model,example-model/2/variables/variables.index,0abf72f415629ad93f62c769ea97f3e6
$ mc cp index-${index_uuid}.csv serving/test-bucket/index-${index_uuid}.csv
```

Once the index file has been processed by the server you will be able to observe the TFX serving configuration data updated as follows:

```
$ kubectl --namespace serving-bridge describe configmap tfx-config
Name:         tfx-config
Namespace:    serving-bridge
Labels:       <none>
Annotations:  <none>

Data
====
tfx-config:
----
model_config_list:  {
  config:  {
    base_path:  "example-model"
    model_platform:  "tensorflow"
  }
}

Events:  <none>
```

###  TFXModel Serving configuration

The last step involves a deployed TFX ModelServer that has been configured to watch a top level directory into which model directory hierarchies will be copied.

The model serving configuration file is scanned on a regular basis by the model server and can be modified to reference new model directories.  The configuration file is in our case provisioned using a Kubernetes ConfigMap that is mounted into the TFX Serving pods.  The mounted location is then configured using the TFX serving --model\_config\_file, and --model\_config\_file\_poll\_wait\_seconds options.

The model server will poll the version directory for new model versions and load them as needed.

Served model inferencing is be done using the standard TensorFlow library, tensorflow\_serving.apis.prediction\_service\_pb2\_grpc, for Javascript there is a REST API to call the prediction service [Client API (REST)](https://www.tensorflow.org/tfx/serving/api_rest), and for Go there is gRPC/ProtoBuf support so the prediction service can be used in this case as well.

For an introduction about deployment of the TFX model serving using Kubernetes please see [Use TensorFlow Serving with Kubernetes](https://www.tensorflow.org/tfx/serving/serving\_kubernetes).

In order to run serving for our case the base tensorflow-model-server software supports the use of S3 within model base path specifications, and the standard AWS environment variables.  It should be noted however that the server on a per pod basis only supports a single set of AWS credentials.  More information concerning the use of S3 for model storage can be found at, https://www.kubeflow.org/docs/components/serving/tfserving\_new/#pointing-to-the-model.

To secure the model service it is recommended that as gateway is used to implement Auth0 or a similar platform for AAA functionality.  For examples on this the Kubeflow serving document has an example of using basic gcloud, for an example of using AAA in a general setting to secure services refer to [Platform Services Example](https://github.com/leaf-ai/platform-services).

If your serving needs require elastic use of resources then the new beta of [KFServing usin knative and Istio](https://www.kubeflow.org/docs/components/serving/kfserving/) should be considered as it is a clear contender for the serving crown.  This solution extends TFX serving for TensorRT, ONNX, XGBoost, SKLearn and PyTorch amoung others.

### Export to Serving Bridge

In order to make models available to the TFX model serving there are two steps involved.

Firstly the model is exported to a well known location. Secondly the model serving configuration file is updated, if the model is completely new or if only specific versions were previously permitted in the configuration.

The serving configuration update however requires that a third software component, or daemon is present to observe the model upload completing and then update the serving configuration file.  The export to serving bridge (bridge) fulfills this role.

Using S3 shared storage the model data is visible to both the export and the serving TFX components.  The bridge will inspect the contents of the specified S3 bucket used in serving models and will on observing the presence of an index file, that is validated against the models directory structure, modify the TFX model serving configuration file to advertise the presence of the model.

The default serving configuration file is called 'models.config'.

The bridge scans for index files in the S3 file system on a regular basis and updates a TFX model serving configuration file found at the top level directory of the mount based on the directories found in the buckets top level.

The bridge along with deployment instructions can be found at [studio-go-runner/tools/serving-bridge](https://github.com/leaf-ai/studio-go-runner/tree/main/tools/serving-bridge)

Copyright © 2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
