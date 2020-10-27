# Production TensorFlow Model Serving

This application note addresses a common question about which model serving offering should be used once a model is trained using StudioML.

There are a large variety of ways to serve models each with pros and cons.  This document attempts to assist those executing on production ML model seving projects with a simple approach based upon simple conventions and using TensorFlow eXtensions (TFX).

This document does not seek to address questions concerning model management.  Model management is largely a function of the style of machine learning being undertaken and the level to which a choosen learning strategy requires manual intervention.  Traditional single objective approaches requirements for model management can be very different than for those using evolutionary multiobjective learning.  Because of this variability it is not in-scope for this document but it should be noted that model selection and monitoring addresed by the use of evolutionary multi-object learning an approach should generally seek low friction management that minimizes manual gatekeeping workflows typical of traditional approaches.

<!--ts-->

Table of Contents
=================

* [Production TensorFlow Model Serving](#production-tensorflow-model-serving)
* [Table of Contents](#table-of-contents)
  * [Introduction](#introduction)
  * [TensorFlow Serving platform](#tensorflow-serving-platform)
  * [TensorFlow Serving Simple Workflow](#tensorflow-serving-simple-workflow)
    * [TFX Model Export](#tfx-model-export)
    * [TFX Model Serving](#tfx-model-serving)
    * [Export to Serving Bridge](#export-to-serving-bridge)
<!--te-->

## Introduction

TensorFlow model serving options have changed significantly over time.  You should expect that the choosen approach we currently use will change at some point in time, possibly quite quickly so this document should function as a starting point.

## TensorFlow Serving platform

TensorFlow model serving is part of the Google production scale machine learning platform known as TFX.  Adopting the serving functionality does not require the wholesale adoption of Googles platform.  StudioML is an automatted Evolutionary ML service for model creation that is an automatted alternative to data scientist orchestrated ML workflows such as KubeFlow.  In both cases training steps result in model files that are stored at a well known location and can then be discovered by the model serving which can then discover and load them automatically.

The TFX based model serving solution is designed to run in both standalone workstation, Docker, and Kubernetes deployments.  The solutions can also be deployed without being coupled to components such as a database or a specific MLops framework.  In addition TFX model serving also offers opportunities for serving other types of embeddings, non TFX models, and data.  In the case of non TensorFlow models custom software needs to be implemeted which is not discussed in this document, this requires C++ chops and is documented here, [Creating a new kind of servable](https://www.tensorflow.org/tfx/serving/custom_servable).

Packaging for the model serving component is container based and offers both Docker Desktop and Kubernetes deployments.  The configuration options for serving allow local and remote storage locations to be automatically served.

## TensorFlow Serving Simple Workflow

TensorFlow Serving consists of two components, export and serving.  We propose a third component that can bridge the two when Kubernetes based deployments are present.

### TFX Model Export

The first step is to train the model and export it using the [TensorFlow SavedModel format](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/python/saved_model/README.md).  Upon export the model and additional assets are saved into a [directory heirarchy](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/python/saved_model/README.md#components) with the top level directory being the version number of the model.

The model export action is typically performed once the evolutionary learning has completed, or converged.

Once the experiment is complete and ready for deployment the experiment can use the python code to invoke the TensorFlow APIs for model export, [Train and export TensorFlow model](https://www.tensorflow.org/tfx/serving/serving_basic#train_and_export_tensorflow_model).

Once exported the experimenter implementing the python orchestration for their experiment then copies the exported directory tree for the model to S3.  In the case where the experimenter has control over the python code used to orchestrate and export the model then boto3 can be used to load the model files as blobs to S3.  Alternatively the model can be copied up using an S3 aware file copy tool such as the [Minio client](https://docs.min.io/docs/minio-client-quickstart-guide.html), or the [AWS CLI S3 reference](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/s3/index.html).

If model promotion is not automatted, on inspection of fitness metrics about the multi-objective trade-offs made experimenters can use a free standing python program to invoke these APIs to perform the export as well.

When the recursive copy is complete the exporter should write a CSV index file into the top level directory of the bucket that is named using a UUID with a prefix of 'index-'.  The contents of the file should be the list of individual files/keys that must be present and observable to the serving before loading the experiments.  Each line in the CSV should have 3 fields, first is the base path for the model, secondly the fully qualified key of the blobs, or file name, and the third field should be the etag (S3 internally defined checksum) of the blob or file checksum.


### TFX Model Serving

The second step involves a deployed TFX ModelServer that has been configured to watch a top level directory into which model directory hierarchies will be copied.

The model serving configuration file is scanned on a regular basis by the model server and can be modified to reference new model directories.  The configuration file is in our case provisioned using a Kubernetes ConfigMap that is mounted into the TFX Serving pods.  The mounted location is then configured using the TFX serving --model_config_file, and --model_config_file_poll_wait_seconds options.

The model server will poll the version directory for new model versions and load them as needed.

Served model inferencing is be done using the standard TensorFlow library, tensorflow\_serving.apis.prediction\_service\_pb2\_grpc, for Javascript there is a REST API to call the prediction service [Client API (REST)](https://www.tensorflow.org/tfx/serving/api_rest), and for Go there is gRPC/ProtoBuf support so the prediction service can be used in this case as well.

For an introduction about deployment of the TFX model serving using Kubernetes please see [Use TensorFlow Serving with Kubernetes](https://www.tensorflow.org/tfx/serving/serving\_kubernetes).

In order to run serving for our case the base tensorflow-model-server software supports the use of S3 within model base path specifications, and the standard AWS environment variables.  It should be noted however that the server on a per pod basis only supports a single set of AWS credentials.  More information concernin the use of S3 for model storage can be found at, https://www.kubeflow.org/docs/components/serving/tfserving_new/#pointing-to-the-model.

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
