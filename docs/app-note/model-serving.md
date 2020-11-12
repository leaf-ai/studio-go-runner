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
      * [Example configuration access](#example-configuration-access)
    * [TFXModel Serving configuration](#tfxmodel-serving-configuration)
      * [Additional TFX Serving notes](#additional-tfx-serving-notes)
<!--te-->

## Introduction

TensorFlow model serving options have changed significantly over time.  You should expect that the choosen approach we currently use will change at some point in time, possibly quite quickly so this document should function as a starting point.

This document details how models being uploaded to S3 can be pre-processed by a bridge, which then generates a TFX serving configuration file that is provisioned using a Kubernetes Config Map that is mounted into a TFX model serving Kubernetes pod and is used as its configuration.

This document describes a general purpose approach to model serving that uses a stock serving container that dynamically loads the models needed for prediction rather than creating a container image for each and every model.  Serving is triggered using a configuration data block that leverages S3 to download the models to be served dynamically.

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

In order to make models available to the TFX model serving there are two steps involved.

Firstly the model is exported to a well known location. Secondly the model serving configuration file is updated, if the model is completely new or if only specific versions were previously permitted in the configuration.

The serving configuration update however requires that a third software component, or daemon is present to observe the model upload completing and then update the serving configuration file.  The export to serving bridge (bridge) fulfills this role.

Using S3 shared storage the model data is visible to both the export and the serving TFX components.  The bridge will inspect the contents of the specified S3 bucket used in serving models and will on observing the presence of an index file, that is validated against the models directory structure, modify the TFX model serving configuration file to advertise the presence of the model.

The bridge scans for index files in the S3 file system on a regular basis and updates a TFX model serving configuration file found at the top level directory of the mount based on the directories found in the buckets top level.

The bridge along with deployment instructions can be found at [studio-go-runner/tools/serving-bridge](https://github.com/leaf-ai/studio-go-runner/tree/main/tools/serving-bridge)
The next step in the pipeline uses an open source component that can be found in the go runner repository at, https://github.com/leaf-ai/studio-go-runner/tree/main/tools/serving-bridge.

This component can be deployed using a Kubernetes deployment resource.  There is a single deployment dependency for the serving bridge and that is the repsence of an S3 blob store to hold models, for cases involving testing and where a local S3 store is required for on-premises installation minio, https://min.io, can be used.

Should you be running in an on-premises environment minio can be substituted for the AWS, or Google cloud storage.  An example of a minio deployment can be found in the minio.yaml file, this example creates a persistent volume for the minio server with 10Gb of storage.

The main deployment file for the bridge can be found in deployment.yaml.  The deployment file makes use of the Kubernetes Kustomize tool for injection of secrets, namespaces and other attributes for inclusion into the deployment configuration.  If you are running the bridge in a production environment you should seek to secure secrets using the norms of your enterprise infrastructure.

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
$ export minio_port=`kubectl get services minio-service -o=jsonpath='{.spec.ports[].nodePort}'`
$ mc alias set serving http://127.0.0.1:$minio_port UserUser PasswordPassword
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
$ mc ls serving/test-bucket/example-model --recursive --json | jq -r '.key, .etag' | paste -d ",\n"  - - | awk '{ print "s3://test-bucket/example-model,s3://test-bucket/example-model/" $0; }' > index-${index_uuid}.csv
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
    name:  "example-model"
    base_path:  "s3://test-bucket/example-model"
    model_platform:  "tensorflow"
  }
}

Events:  <none>
```

The tensorflow serving pod is the standard Google supplied image for tensorflow serving and can be accessed if needed using the following command:

```
kubectl exec -it `kubectl get pods --selector=app=tfx-serving -o=jsonpath="{.items[0].metadata.name}"` -- /bin/bash
```

###  TFXModel Serving

The last step involves a deployed TFX ModelServer that has been configured to watch a Kubernetes provisioned configuration map for changes that indicated models which should be loaded or discarded.

The model serving configuration is scanned on a regular basis by the model server and can be modified to reference new model S3 keys.  The configuration file is in our case provisioned using a Kubernetes ConfigMap that is mounted into the TFX Serving pods.  The mounted location is then configured using the TFX serving --model\_config\_file, and --model\_config\_file\_poll\_wait\_seconds options.

The model server will poll the version directory for new model versions and load them as needed.

Served model inferencing is be done using the standard TensorFlow library, tensorflow\_serving.apis.prediction\_service\_pb2\_grpc, for Javascript there is a REST API to call the prediction service [Client API (REST)](https://www.tensorflow.org/tfx/serving/api_rest), and for Go there is gRPC/ProtoBuf support so the prediction service can be used in this case as well.

For an introduction about deployment of the TFX model serving using Kubernetes please see [Use TensorFlow Serving with Kubernetes](https://www.tensorflow.org/tfx/serving/serving\_kubernetes).

The TFX model server is configured with several standard TCP/IP ports, the /usr/bin/tensorflow_model_server executable and its command line options.  The two TCP/IP ports 8500, and 8501 for REST and gRPC respectively.  In the default container image, tensorflow/serving:2.3.0, the configuration calls for a single model in the /models/model directory. The serving.yaml file contains the Kubenetes configuration for the standard container image and initializing it a modified configuration of the standard configuration to one using a configuration file rather than a fixed model.

In order to run serving for our case the base tensorflow-model-server software supports the use of S3 within model base path specifications, and the standard AWS environment variables.  It should be noted however that the server on a per pod basis only supports a single set of AWS credentials.  More information concerning the use of S3 for model storage can be found at, https://www.kubeflow.org/docs/components/serving/tfserving_new/#pointing-to-the-model.

The example configuration for the serving can be found in the tools/serving-bridge/serving.yaml file.  If you have used Kustomize to deploy then you can gain access to the logs for the serving using a command such as the following:

```
$ kubectl logs --namespace serving-bridge --selector=app=tfx-serving -f
2020-11-10 18:39:01.407045: I tensorflow_serving/model_servers/server_core.cc:464] Adding/updating models.
2020-11-10 18:39:01.410196: I tensorflow_serving/model_servers/server_core.cc:464] Adding/updating models.
2020-11-10 18:39:01.411314: I tensorflow_serving/model_servers/server.cc:367] Running gRPC ModelServer at 0.0.0.0:8500 ...
2020-11-10 18:39:01.413039: I tensorflow_serving/model_servers/server.cc:387] Exporting HTTP/REST API at:localhost:8501 ...
[evhttp_server.cc : 238] NET_LOG: Entering the event loop ...
2020-11-10 18:40:01.410264: I tensorflow_serving/model_servers/server_core.cc:464] Adding/updating models.
2020-11-10 18:41:01.410380: I tensorflow_serving/model_servers/server_core.cc:464] Adding/updating models.
2020-11-10 18:41:01.410414: I tensorflow_serving/model_servers/server_core.cc:575]  (Re-)adding model: example-model
2020-11-10 18:41:01.517087: I tensorflow_serving/core/basic_manager.cc:739] Successfully reserved resources to load servable {name: example-model version: 2}
2020-11-10 18:41:01.517120: I tensorflow_serving/core/loader_harness.cc:66] Approving load for servable version {name: example-model version: 2}
2020-11-10 18:41:01.517132: I tensorflow_serving/core/loader_harness.cc:74] Loading servable version {name: example-model version: 2}
2020-11-10 18:41:01.520328: I external/org_tensorflow/tensorflow/cc/saved_model/reader.cc:31] Reading SavedModel from: s3://test-bucket/example-model/2
2020-11-10 18:41:01.531353: I external/org_tensorflow/tensorflow/cc/saved_model/reader.cc:54] Reading meta graph with tags { serve }
2020-11-10 18:41:01.531393: I external/org_tensorflow/tensorflow/cc/saved_model/loader.cc:234] Reading SavedModel debug info (if present) from: s3://test-bucke
t/example-model/2
2020-11-10 18:41:01.533016: I external/org_tensorflow/tensorflow/core/platform/cpu_feature_guard.cc:142] This TensorFlow binary is optimized with oneAPI Deep N
eural Network Library (oneDNN)to use the following CPU instructions in performance-critical operations:  AVX2 AVX512F FMA
To enable them in other operations, rebuild TensorFlow with the appropriate compiler flags.
2020-11-10 18:41:01.556999: I external/org_tensorflow/tensorflow/cc/saved_model/loader.cc:199] Restoring SavedModel bundle.
2020-11-10 18:41:01.619074: I external/org_tensorflow/tensorflow/cc/saved_model/loader.cc:183] Running initialization op on SavedModel bundle at path: s3://tes
t-bucket/example-model/2
2020-11-10 18:41:01.626019: I external/org_tensorflow/tensorflow/cc/saved_model/loader.cc:303] SavedModel load for tags { serve }; Status: success: OK. Took 10
5690 microseconds.
2020-11-10 18:41:01.629152: I tensorflow_serving/servables/tensorflow/saved_model_warmup_util.cc:59] No warmup data file found at s3://test-bucket/example-mode
l/2/assets.extra/tf_serving_warmup_requests
2020-11-10 18:41:01.661964: I tensorflow_serving/core/loader_harness.cc:87] Successfully loaded servable version {name: example-model version: 2}
2020-11-10 18:42:01.410475: I tensorflow_serving/model_servers/server_core.cc:464] Adding/updating models.
2020-11-10 18:42:01.410515: I tensorflow_serving/model_servers/server_core.cc:575]  (Re-)adding model: example-model
2020-11-10 18:43:01.410562: I tensorflow_serving/model_servers/server_core.cc:464] Adding/updating models.
2020-11-10 18:43:01.410604: I tensorflow_serving/model_servers/server_core.cc:575]  (Re-)adding model: example-model
2020-11-10 18:44:01.410651: I tensorflow_serving/model_servers/server_core.cc:464] Adding/updating models.
```

Now that the model is being served the next step is to identify the model serving endpoint against which python and other prediction/classification clients can make their requests.  To do this the kubectl command can be used to identify the endpoint port number. For example:

```
$ kubectl --namespace serving-bridge get services tfx-serving
NAME      TYPE           CLUSTER-IP      EXTERNAL-IP   PORT(S)                         AGE
tfx-serving   LoadBalancer   10.152.183.21   <pending>     8501:32412/TCP,8500:31492/TCP   162m
```

This command shows that the REST interface on 8501 has been mapped to the port number 32412, and the gRPC interface can be found on port 31492.  In the above case the service is running on your local host meaning that the REST predictions can be directed at 127.0.0.1:32412.

```
python model_gen/classify.py --port=32412
2020-11-11 17:54:11.129867: W tensorflow/stream_executor/platform/default/dso_loader.cc:55] Could not load dynamic library 'libnvinfer.so.6'; dlerror: libnvinfer.so.6: cannot open shared object file: No such file or directory
2020-11-11 17:54:11.130139: W tensorflow/stream_executor/platform/default/dso_loader.cc:55] Could not load dynamic library 'libnvinfer_plugin.so.6'; dlerror: libnvinfer_plugin.so.6: cannot open shared object file: No such file or directory
2020-11-11 17:54:11.130271: W tensorflow/compiler/tf2tensorrt/utils/py_utils.cc:30] Cannot dlopen some TensorRT libraries. If you would like to use Nvidia GPU with TensorRT, please make sure the missing libraries mentioned above are installed properly.
TensorFlow version: 2.1.0
{'predictions': [[0.00182419526, 1.38313e-09, 0.979982734, 0.000220539834, 0.00282438565, 3.32592677e-13, 0.0151464706, 2.41131031e-15, 1.6579487e-06, 3.86284049e-13]]}
```

The result of running the classification is an array of scores of each of the possible labels.The code used to generate the prediction is summerized as follos:

```python
# A minimal model predictor that can be used to access remote test models
# used for general testing but which are not intended to be of
# much utility for making valuable predictions
#
import tensorflow as tf
from tensorflow import keras

import json
import requests

import argparse

parser = argparse.ArgumentParser(description='Perform model predictions')
parser.add_argument('--port', dest='port', help='port is the TCP port for the prediction endpoint')

args = parser.parse_args()

print('TensorFlow version: {}'.format(tf.__version__))
fashion_mnist = keras.datasets.fashion_mnist
(train_images, train_labels), (test_images, test_labels) = fashion_mnist.load_data()

# scale the values to 0.0 to 1.0
test_images = test_images / 255.0

# reshape for feeding into the model
test_images = test_images.reshape(test_images.shape[0], 28, 28, 1)

# Grab an image from the test dataset and then
# create a json string to ask query to the depoyed model
data = json.dumps({"signature_name": "serving_default",
    "instances": test_images[1:2].tolist()})

# headers for the post request
headers = {"content-type": "application/json"}

# make the post request
json_response = requests.post(f'http://127.0.0.1:{args.port}/v1/models/example-model/versions/2:predict',
                              data=data,
                              headers=headers)

# get the predictions
predictions = json.loads(json_response.text)
print(predictions)
```
All of the code can be found in the tools/serving-bridge/model_gen directory and the classify.py file.

#### Additional TFX Serving notes

To secure the model service it is recommended that as gateway is used to implement Auth0 or a similar platform for AAA functionality.  For examples on this the Kubeflow serving document has an example of using basic gcloud, for an example of using AAA in a general setting to secure services refer to [Platform Services Example](https://github.com/leaf-ai/platform-services).

The model server can be used within an Istio mesh which is recommended for advanced configurations.

If your serving needs require elastic use of resources then the new beta of [KFServing using knative and Istio](https://www.kubeflow.org/docs/components/serving/kfserving/) should be considered as it is a clear contender for the serving crown.  This solution extends TFX serving for TensorRT, ONNX, XGBoost, SKLearn and PyTorch amoung others.

Copyright © 2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
