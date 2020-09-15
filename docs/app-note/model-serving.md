# Production TensorFlow Model Serving

This application note addresses a common question about that specific model serving offering that should be used once a model is trained using StudioML.

There are many, many ways of serving models and as expected each has their pros and cons.  This document attempts to assist those executing on production ML model seving projects with a simple approach based upon simple conventions and using TensorFlow eXtensions (TFX).

<!--ts-->

Table of Contents
=================

* [Production TensorFlow Model Serving](#production-tensorflow-model-serving)
  * [Introduction](#introduction)
  * [TensorFlow Serving platform](#tensorflow-serving-platform)
  * [TensorFlow Serving Simple Workflow](#tensorflow-serving-simple-workflow)
    * [TFX Model Export](#tfx-model-export)
    * [TFX Model Serving](#tfx-model-serving)
    * [Export to Serving Bridge](#export-to-serving-bridge)
<!--te-->

## Introduction

Approaches to TensorFlow model serving have changed significantly over time.  You should expect that the choosen approach we currently use will change at some point in time, possibly quite quickly so this document should function as a starting point.

## TensorFlow Serving platform

TensorFlow model serving is part of Googles production scale machine learning platform known as TFX.  Adopting the serving functionality does not require the wholesale adoption of Googles platform.  StudioML is an automatted Evolutionary ML service for model creation that is a fully automatted alternative to data scientist orchestrated ML workflows such as KubeFlow.  in both cases training steps result in model files that are stored at a well known location and then be discovered by the model serving which can then discover and load them automatically.

The TFX based model serving solution is designed to run in both standalone workstation, Docker, and Kubernetes deployments.  The solutions can also be deployed without being coupled to components such as a database or a specific MLops framework.  In addition TFX model serving also offers opportunities for serving other types of embeddings, non TFX models, and data.  In the case of non TensorFlow models custom software needs to be implemeted which is not discussed in this document.

Packaging for the model serving component is container based and offers both Docker Desktop and Kubernetes deployments.  The configuration options for serving allow local and remote storage locations to be automatically served.

## TensorFlow Serving Simple Workflow

TensorFlow Serving consists of xxx components.

### TFX Model Export

The first step is to train the model and export it using the [TensorFlow SavedModel format](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/python/saved_model/README.md).  Upon export the model and additional assets are saved into a [directory heirarchy](https://github.com/tensorflow/tensorflow/blob/master/tensorflow/python/saved_model/README.md#components) with the top level directory being the version number of the model.

The model export set is typically performed once the evolutionary learning has completed, or converged.

Once ths experiment is complete and ready for deployment the experiment can use the python code to invoke the TensorFlow APIs for model export, [Train and export TensorFlow model](https://www.tensorflow.org/tfx/serving/serving_basic#train_and_export_tensorflow_model).

Once exported the experimenter implementing the python orchestration for their experiment then pushes the exported directory tree for the model to S3.  In the case where the experimenters host contains the S3 Fuse driver they can write to a local mount for the S3 bucket.  Alternatively the model can be copied up using an S3 aware file copy tool such as the [Minio client](https://docs.min.io/docs/minio-client-quickstart-guide.html), or the [AWS CLI S3 reference](https://awscli.amazonaws.com/v2/documentation/api/latest/reference/s3/index.html).

Model transfers for deployment can be automatted using a directory copy within the python experiment orchestration code.  If not automatted on inspection of metrics about the trade-offs made and the resulting fitness experimenters can use a free standing python program to invoke these APIs to perform the export.

Once the recursive copy is complete the exporter should write a CSV index file into the top level directory of the bucket that is named using a UUID with a prefix of 'index-'.  The contents of the file should be the list of individual files/keys that must be present and observable to the serving before loading the experiment.s  Each line in the CSV should have 2 fields, first the item/key identifing blobs, or files and the second field should be the length of the file.

### TFX Model Serving

The second step involves a deployed TFX ModelServer that has been configured to watch a top level directory into which model directory hierarchies will be copied. The model serving configuration file is scanned on a regular basis by the model server and can be modified to reference new model directories.
The model server will poll the version directory for new model versions and load them as needed.

Once models are served inferencing can be done using the standard TensorFlow library, tensorflow\_serving.apis.prediction\_service\_pb2\_grpc, for Javascript there is a REST API to call the prediction service [Client API (REST)](https://www.tensorflow.org/tfx/serving/api_rest), and for Go there is gRPC/ProtoBuf support so the prediction service can be used in this case as well.

For an introduction about deployment of the TFX model serving using Kubernetes please see [Use TensorFlow Serving with Kubernetes](https://www.tensorflow.org/tfx/serving/serving\_kubernetes).

In order to run serving for our case the S3 fuse driver will need to be added to the Dockerfile as also described in the following sub section.

### Export to Serving Bridge

In order to make models available to the TFX model serving there are two steps involved.

Firstly the model is exported to a well known location. Secondly the model serving configuration file is updated, if the model is completely new or if only specific versions were previously permitted in the configuration.

Using Kubernetes shared storage the model data is visible to both the export and the serving TFX components.  Detailed information as to how to use S3 as a mountable resource please review [Mouting S3 bucket in docker containers on kubernetes](https://blog.meain.io/2020/mounting-s3-bucket-kube/).  An option does exist for native AWS users and that is using the AWS Kubernetes EFS storage provider as documented in [Kubernetes: Shared storage volume between multiple pods on Amazon.](https://blog.abyssale.com/shared-storage-volume-on-amazon/).  While this application note describes the use of an open source fuse based approach for mounting S3 buckets there is a higher performance offering available called [ObjectiveFS](https://objectivefs.com/) which we have not tried but does promise better performance.

The serving configuration update however requires that a third software component, or daemon is present to observe the model upload completing and then update the serving configuration file.  The export to serving bridge (bridge) fulfills these roles.

The bridge examines the shared mounted file system on a regular basis and updates a TFX model serving confiuration file found at the top level directory of the mount based on the directories found in the mounts top level directory.

The default serving configuration file is called 'models.config'.

The bridge along with deployment instructions can be found at [studio-go-runner/tools/serving-bridge](https://github.com/leaf-ai/studio-go-runner/tree/main/tools/serving-bridge)

Copyright © 2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
