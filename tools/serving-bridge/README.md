# TensorFlow Model Export to Serving bridge

This tool is motivated by a need to promote machine learning models for serving using TFX model serving.

For detailed information about this tools role within model serving infrastructure please read the [Production TensorFlow Model Serving Application Note](../../docs/app-note/model-serving.md).

<!--ts-->

Table of Contents
=================

* [TensorFlow Model Export to Serving bridge](#tensorflow-model-export-to-serving-bridge)
* [Table of Contents](#table-of-contents)
  * [Introduction](#introduction)
<!--te-->

## Introduction

The model export to serving bridge is a daemon deployed within Kubernetes for watching S3 blobs identified as indexes to models present within a bucket and updating a TFX model server configuration file to activate model serving.

This software component is desined to be deployed as part of an exported model to model serving pipeline that is entirely automatted.

Copyright © 2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
