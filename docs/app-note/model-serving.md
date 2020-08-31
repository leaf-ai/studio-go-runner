# Production TensorFlow Model Serving

This application note addresses a common question about which model serving offering should be used once a model is trained using StudioML.

There are many, many ways of serving models and as expected each has their pros and cons.  This document attempts to assist those executing on production ML model seving projects with a strawman approach.

<!--ts-->
<!--te-->

## Introduction

The approaches to TensorFlow model serving have changed significantly over time.  Likewise we expect that the choosen approach we currently use would also change at some point in time, possibly quite quickly so this document should function as a starting point.

## TensorFlow serving platform

The TensorFlow model serving solution is designed to run in both standalone workstation, Docker, and Kubernetes environments.  The solutions can also be deployed without being coupled to other components such as a database or a specific MLops framework.  It also offers opportunities for serving other types of models and data.

TensorFlow serving is provided as part of Googles production scale platform called TFX.  Adopting the serving functionality does not require the wholesale adoption of Googles platform.  StudioML is an automatted Evolutionary ML service that complements data scientist orchestrated ML workflows such as KubeFlow.  Pipeline training steps result in model files that can be stored at a well known location and then be discovered by the TensorFlow serving which can then load them automatically.

Packaging for the serving module is container based and offers both Docker Desktop and Kubernetes deployments.  The configuration options for serving allow local and remote storage locations to be automatically served.

Copyright © 2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
