# Interfacing and Integration

This document describes the interface, and interchange format used between the StudioML client and runners that process StudioML experiments.

## Introduction

StudioML has two major modules.

. The client, or front end, that shepherds experiments on behalf of users and packaging up experiments that are then placed on to a queue using json messages

. The runner that receives json formatted messages on a message queue and then runs the experiment they describe

There are other tools that StudioML offers for reporting and management of experiment artifacts that are not within the scope of this document.

It is not yet within the scope of this document to describe how data outside of the queuing interface is stored and formatted.

## Audience

This document is intended for developers who wish to implement runners to process StudioML work, or implement clients that generate work for StudioML runners.

## Runners

This project implements a StudioML runner, however it is not specific to StudioML.  This runner could be used to deliver and execute and python code within a virtualenv that the runner supplies.

Any standard runners can accept a standalone virtualenv with no associated container.  The go runner, this present project, has been extended to allow clients to also send work that has a Singularity container specified.

In the first case, virtualenv only, the runner implcitly trusts that any work received is trusted and is not malicous.  In this mode the runner makes not attempt to protect the integrity of the host it is deployed into.

In the second case if a container is specified it will be used to launch work and the runner will rely upon the container runtime to prevent leakage into the host.

## Queuing

The StudioML eco system relies upon a message queue to buffer work being sent by the StudioML client to any arbitrary runner that is subscribed to the experimenters choosen queuing service.  StudioML support multiple queuing technologies including, AWS SQS, Google PubSub, local file system, and RabbitMQ.  The reference implementation is RabbitMQ for the purposes of this present project.

Additional queuing technologies can be added if desired to the StudioML (https://github.com/studioml/studio.git), and go runner (https://github.com/SentientTechnologies/studio-go-runner.git) code bases and a pull request submitted.

When using a queue the StudioML eco system relies upon a reliable, at-least-once, messaging system.  An additional requirement for queuing systems is that if the worker disappears, or work is not reclaimed by the worker as progress is made that the work is requeued by the broker automatically.

## Experiment Lifecycle

If you have had a chance to run some of the example experiments within the StudioML github repository then you will have noticed a keras example.  The keras example is used to initiate a single experiment that queues work for a single runner and then immediately returns to the command line prompt without waiting for a result.  Experiments run in this way rely on the user to monitor their cloud storage bucket and look for the output.tar file in a directory named after their experiment.  For simple examples and tests this is a quick but manual way to work.

In more complex experiments there might be multiple phases to a project that is being run.  Each experiment might represent an individual in for example evolutionary computation.  The python software running the project might want to send potentially hundreds of experiments, or individuals to the runners and then wait for these to complete.  Once complete it might select individuals that scored highly, using as one example a fitness screen.  The python StudioML client might then generate a new population that are then marshall individuals from the population into experiments, repeating this cycle potentially for days.

To address the need for longer running experiments StudioML offers a number of python classes within the open source distribution that allows this style of longer running taining scenarios to be implemented by researchers and engineers.  The combination of completion service and session server classes can be used to create these long running StudioML compliant clients.

Completion service based applications that use the StudioML classes generate work in exactly the same way as the CLI based 'studio run' command.  Session servers are an implementation of a completion service combined with logic that once experiments are queued will on a regular interval examine the cloud storage folders for returned archives that runners have rolled up when they either save experiment workspaces, or at the conclusion of the experiment find that the python experiment code had generated files in directories identified as a part of the queued job.  After the requisite numer of experiments are deemed to have finished based on the storage server bucket contents the session server can then examine the uploaded artifacts and determine their next set of training steps.

## Payloads

The following figure shows an example of a job sent from the studioML front end to the runner.  The runner does not always make use of the entire set of json tags, typically a limited but consistent subset of tags are used.

```json
{
  "experiment": {
    "status": "waiting",
    "time_finished": null,
    "git": null,
    "key": "1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92",
    "time_last_checkpoint": 1530054414.027222,
    "pythonver": "2.7",
    "metric": null,
    "args": [
      "10"
    ],
    "max_duration": "20m",
    "filename": "train_cifar10.py",
    "project": null,
    "artifacts": {
      "output": {
        "local": "/home/kmutch/.studioml/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/output",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/output.tar",
        "key": "experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/output.tar",
        "mutable": true,
        "unpack": true
      },
      "_metrics": {
        "local": "/home/kmutch/.studioml/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/_metrics",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/_metrics.tar",
        "key": "experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/_metrics.tar",
        "mutable": true,
        "unpack": true
      },
      "modeldir": {
        "local": "/home/kmutch/.studioml/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/modeldir",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/modeldir.tar",
        "key": "experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/modeldir.tar",
        "mutable": true,
        "unpack": true
      },
      "workspace": {
        "local": "/home/kmutch/studio/examples/keras",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/blobstore/419411b17e9c851852735901a17bd6d20188cee30a0b589f1bf1ca5b487930b5.tar
",
        "key": "blobstore/419411b17e9c851852735901a17bd6d20188cee30a0b589f1bf1ca5b487930b5.tar",
        "mutable": false,
        "unpack": true
      },
      "tb": {
        "local": "/home/kmutch/.studioml/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/tb",
        "bucket": "kmutch-rmq",
        "qualified": "s3://s3-us-west-2.amazonaws.com/kmutch-rmq/experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/tb.tar",
        "key": "experiments/1530054412_70d7eaf4-3ce3-493a-a8f6-ffa0212a5c92/tb.tar",
        "mutable": true,
        "unpack": true
      }
     "info": {},
    "resources_needed": {
      "hdd": "3gb",
      "gpus": 1,
      "ram": "2gb",
      "cpus": 1,
      "gpuMem": "4gb"
    },
    "pythonenv": [
      "APScheduler==3.5.1",
      "argparse==1.2.1",
      "asn1crypto==0.24.0",
      "attrs==17.4.0",
      "autopep8==1.3.5",
      "awscli==1.15.4",
      "boto3==1.7.4",
      "botocore==1.10.4",
...
      "six==1.11.0",
      "sseclient==0.0.19",
      "-e git+https://github.com/SentientTechnologies/studio@685f4891764227a2e1ea5f7fc91b31dcf3557647#egg=studioml",
      "terminaltables==3.1.0",
      "timeout-decorator==0.4.0",
      "tzlocal==1.5.1",
      "uritemplate==3.0.0",
      "urllib3==1.22",
      "Werkzeug==0.14.1",
      "wheel==0.31.0",
      "wsgiref==0.1.2"
    ],
    "owner": "guest",
    "time_added": 1530054413.134781,
    "time_started": null
  },
  "config": {
    "experimentLifetime": "30m",
    "optimizer": {
      "visualization": true,
      "load_checkpoint_file": null,
      "cmaes_config": {
        "load_best_only": false,
        "popsize": 100,
        "sigma0": 0.25
      },
      "termination_criterion": {
        "generation": 5,
        "fitness": 999,
        "skip_gen_timeout": 30,
        "skip_gen_thres": 1
      },
      },
      "result_dir": "~/Desktop/",
      "checkpoint_interval": 0
    },
    "verbose": "debug",
    "saveWorkspaceFrequency": "3m",
    "database": {
      "type": "s3",
      "authentication": "none",
      "endpoint": "http://s3-us-west-2.amazonaws.com",
      "bucket": "kmutch-metadata"
    },
    "runner": {
      "slack_destination": "@karl.mutch"
    },
    "storage": {
      "type": "s3",
      "endpoint": "http://s3-us-west-2.amazonaws.com",
      "bucket": "kmutch-rmq"
    },
    "server": {
      "authentication": "None"
    },
    "resources_needed": {
      "gpus": 1,
      "hdd": "3gb",
      "ram": "2gb",
      "cpus": 1,
      "gpuMem": "4gb"
    },
    "env": {
      "PATH": "%PATH%:./bin",
      "AWS_DEFAULT_REGION": "us-west-2",
      "AWS_ACCESS_KEY_ID": "AKZAIE5G7Q2GZC3OMTYW",
      "AWS_SECRET_ACCESS_KEY": "rt43wqJ/w5aqAPat659gkkYpphnOFxXejsCBq"
    },
    "cloud": {
      "queue": {
        "rmq": "amqp://user:password@10.230.72.19:5672/%2f?connection_attempts=30&retry_delay=.5&socket_timeout=5"
      }
    }
  }
}
```

### experiment ↠ pythonver

The value for this tag must be an integer 2 or 3 for the specific python version requested by the experimenter.

### experiment ↠ args

A list of the command line arguments to be supplied to the python interpreter that will be passed into the main of the running python job.

### experiment ↠ max\_duration

The period of time that the experiment is permitted to run in a single attempt.  If this time is exceeded the runner can abandon the task at any point but it may continue to run for a short period.

### experiment ↠ filename

The python file in which the experiment code is to be found.  This file should exist within the workspace artifact archive relative to the top level directory.

### experiment ↠ project

All experiments should be assigned to a project.  The project identifier is a label assigned by the StudioML user and is specific to their purposes.

### experiment ↠ artifacts

Artifacts are assigned labels, some labels have significance.  The workspace artifact should contain any python code that is needed, it may container other assets for the python code to run including configuration files etc.  The output artifact is used to identify where any logging and returned results will be archives to.

Work that is sent to StudioML runners must have at least one workspace artifact consisting of the python code that will be run.  Artifacts are typically tar archives that contain not just python code but also any other data needed by the experiment being run.

Before the experiment commences the artifact will be unrolled onto local disk of the container running it.  When unrolled the artifact label is used to name the peer directory into which any files are placed.

The experiment when running will be placed into the workspace directory which contains the contents of the workspace labeled artifact.  Any other artifacts that were downloaded will be peer directories of the workspace directory.  Artifacts that were mutable and not available for downloading at the start of the experiment will results in empty peer directories that are named based on the label as well.

Artifacts do not have any restriction on the size of the data they identify.

The StudioML runner will download all artifacts that it can prior to starting an experiment.  Should any mutable artifacts be not available then they will be ignored and the experiment will continue.  If non-mutable artifacts are not found then the experiment will fail.

Named non-mutable artifacts are subject to caching to reduce download times and network load.

### experiment ↠ artifacts ↠ [label] ↠ bucket

The bucket identifies the cloud providers storage service bucket.  This value is not used when the go runner is running tasks.  This value is used by the python runner for configurations where the StudioML client is being run in proximoity to a StudioML configuration file.

### experiment ↠ artifacts ↠ [label] ↠ key

The key identifies the cloud providers storage service key value for the artifact.  This value is not used when the go runner is running tasks.  This value is used by the python runner for configurations where the StudioML client is being run in proxiomity to a StudioML configuration file.

### experiment ↠ artifacts ↠ [label] ↠ qualified

The qualified field contains a fully specified cloud storage platform reference that includes a schema used for selecting the storage platform implementation.  The host name is used within AWS to select the appropriate endpoint and region for the bucket, when using Minio this identifies the endpoint being used including the port number.  The URI path contains the bucket and file name (key in the case of AWS) for the artifact.

If the artifact is mutable and will be returned to the S3 or Minio storage then the bucket MUST exist otherwise the experiment will fail.

The environment section of the json payload is used to supply the needed credentials for the storage.  The go runner will be extended in future to allow the use of a user:password pair inside the URI to allow for multiple credentials on the cloud storage platform.

### experiment ↠ artifacts ↠ [label] ↠ mutable

mutable is a true/false flag for identifying whether an artifact should be returned to the storage platform being used.  mutable artifacts that are not able to be downloaded at the start of an experiment will not cause the runner to terminate the experiment, non-mutable downloads that fail will lead to the experiment stopping.

### experiment ↠ artifacts ↠ [label] ↠ unpack

unpack is a true/false flag that can be used to supress the tar or other compatible archive format archive within the artifact.

### experiment ↠ artifacts ↠ resources\_needed

This section is a repeat of the experiment config resources_needed section, please ignore.

### experiment ↠ artifacts ↠ pythonenv

This section encapsulates a json string array containing pip install dependencies and their versions.  The string elements in this array are a json rendering of what would typically appear in a pip requirements files.  The runner will unpack the frozen pip packages and will install them prior to the experiment running.  Any valid pip reference can be used except for private dependencies that require specialized authentication which is not supported by runners.  If a private dependency is needed then you should add the pip dependency as a file within an artifact and load the dependency in your python experiment implemention to protect it.

### experiment ↠ artifacts ↠  time added

The time that the experiment was initially created expressed as a floating point number representing the seconds since the epoc started, January 1st 1970.

### experiment ↠ config

The StudioML configuration file can be used to store parameters that are not processed by the StudioML client.  These values are passed to the runners and are not validated.  When present to the runner they can then be used to configure it or change its behavior.  If you implement your own runner then you can add values to the configuration file and they will then be placed into the config section of the json payload the runner receives.

Running experiments that make use of Sentient ENN tooling or third party libraries will often require that framework specific configuration values be placed into this section.  Example of frameworks that use these values include the StudioML completion service, and evolutionary strategies used for numerical optimization.

### experiment ↠ config ↠ experimentLifetime

This variable is used to inform the go runner of the date and time that the experiment should be considered to be dead and any work related to it should be abandoned or discarded.  This acts as a gaureentee that the client will no longer need to be concerned with the experiment and work can be requeued in the system, as one example, without fear of repeatition.

The value is expressed as an integer followed by a unit, s,m,h.

### experiment ↠ config ↠ verbose

verbose can be used to adjust the logging level for the runner and for StudioML components.  It has the following valid string values debug, info, warn, error, crit.

### experiment ↠ config ↠ saveWorkspaceFrequency

On a regular basis the runner can upload any logs and intermediate results from the experiments mutable labelled artifact directories.  This variable can be used to set the interval at which these uploads are done.  The primary purpose of this variable is to speed up remote monitoring of intermediate output logging from the runner and the python code within the experiment.

This variable is not intended to be used as a substitute for experiment checkpointing.

### experiment ↠ config ↠ database

The database within StudioML is used to store meta-data that StudioML generates to describe experiments, projects and other useful material related to the progress of experiments such as the start time, owner.

The database can point at blob storage or can be used with structured datastores should you wish to customize it.  The database is used in the event that the API server is launched by a user as a very simply way of accessing experiment and user details.

### experiment ↠ config ↠ database ↠ type

This variable denotes the storage format being used by StudioML to store meta-data and supports three types within the open source offering, firebase, gcloud, s3.  Using s3 does allow other stores such as Azure blob storage when a bridging technology such as Minio is used.

### experiment ↠ config ↠ database ↠ authentication

Not yet widely supported across the database types this variable supports either none, firebase, or github.  Currently its application is only to the gcloud, amnd firebase storage.  The go runner is intended for non vendor dependent implementations and uses the env variable seetings for the AWS authentication currently.  It is planned in the future that the authentication would make use of shortlived tokens using this field.

### experiment ↠ config ↠ database ↠ endpoint

The endpoint variable is used to denote the S3 endpoint that is used to terminate API requests on.  This is used for both native S3 and minio support.  

In the case of a native S3 deployment it will be one of the well known endpoints for S3 and should be biased to using the region specific endpoints for the buckets being used, an example for this use case would be 'http://s3-us-west-2.amazonaws.com'.

In the case of minio this should point at the appropriate endpoint for the minio server along with the port being used, for example http://40.114.110.201:9000/.  If you wish to use HTTPS to increase security the runners deployed must have the appropriate root certificates installed and the certs on your minio server setup to reference one of the publically well known certificate authorities.

### experiment ↠ config ↠ database ↠ bucket

The bucket variable denotes the bucket name being used and should be homed in the region that is configured using the endpoint and any AWS style environment variables captured in the environment variables section, 'env'.

### experiment ↠ config ↠ storage

The storage area within StudioML is used to store the artifacts and assets that are created by the StudioML client.  The typical files placed into the storage are include any directories that are stored on the local workstation of the experimenter and need to be copied to a location that is available to runners.

At a minimum when an experiment starts there will be an workspace artifact placed into the storage area.  Any artifacts placed into the storage will have a key that denotes the exact experiment and the name of the directory that was archived.

Upon completion of the experiment the storage area will be updated with artifacts that are denoted as mutable and that have been changed.

### experiment ↠ config ↠ storage ↠ type

This variable denotes the storage being used as either gs (google cloud storage), or s3.

### experiment ↠ config ↠ storage ↠ endpoint

The endpoint variable is used to denote the S3 endpoint that is used to terminate API requests on.  This is used for both native S3 and minio support.

In the case of a native S3 deployment it will be one of the well known endpoints for S3 and should be biased to using the region specific endpoints for the buckets being used, an example for this use case would be 'http://s3-us-west-2.amazonaws.com'.

In the case of minio this should point at the appropriate endpoint for the minio server along with the port being used, for example http://40.114.110.201:9000/.  If you wish to use HTTPS to increase security the runners deployed must have the appropriate root certificates installed and the certs on your minio server setup to reference one of the publically well known certificate authorities.

### experiment ↠ config ↠ storage ↠ bucket

The bucket variable denotes the bucket name being used and should be homed in the region that is configured using the endpoint.  In the case of AWS any AWS style environment variables captured in the environment variables section, 'env', will be used for authentication.

When the experiment is being initiated within the StudioML client then local AWS environment variables will be used.  When the bucket is accessed by the runner then the authentication details captured inside this json payload will be used to download and upload any data.

### experiment ↠ config ↠ storage ↠ authentication

Not yet widely supported across the database types this variable supports either none, firebase, or github.  Currently its application is only to the gcloud, amnd firebase storage.  The go runner is intended for non vendor dependent implementations and uses the env variable seetings for the AWS authentication currently.  It is planned in the future that the authentication would make use of shortlived tokens using this field.

### experiment ↠ config ↠ resources\_needed

This section details the minimum hardware requirements needed to run the experiment.

Values of the parameters in this section are either integers or integer units.  For units suffixes can include Mb, Gb, Tb for megabytes, gigabytes, or terrabytes.

It should be noted that GPU resources are not virtualized and the requirements are hints to the scheduler only.  A project over committing resources will only affects its own experiments as GPU cards are not shared across projects.  CPU and RAM are virtualized by the container runtime and so are not as prone to abuse.

### experiment ↠ config ↠ resources\_needed ↠ hdd

The minimum disk space required to run the experiment.

### experiment ↠ config ↠ resources\_needed ↠ cpus

The number of CPU Cores that should be available for the experiments.  Remember this value does not account for the power of the CPU.  Consult your cluster operator or administrator for this information and adjust the number of cores to deal with the expectation you have for the hardware.

### experiment ↠ config ↠ resources\_needed ↠ ram

The amount of free CPU RAM that is needed to run the experiment.  It should be noted that StudioML is design to run in a co-operative environment where tasks being sent to runners adequately describe their resource requirements and are scheduled based upon expect consumption.  Runners are free to implement their own strategies to deal with abusers.

### experiment ↠ config ↠ resources\_needed ↠ gpus

gpus are counted as slots using the relative throughput of the physical hardware GPUs. GTX 1060's count as a single slot, GTX1070 is two slots, and a TitanX is considered to be four slots.  GPUs are not virtualized and so the go runner will pack the jobs from one experiment into one GPU device based on the slots.  Cards are not shared between different experiments to prevent noise between projects from affecting other projects.  If a project exceeds its resource consumption promise it will only impact itself.

### experiment ↠ config ↠ resources\_needed ↠ gpuMem

The amount on onboard GPU memory the experiment will require.  Please see above notes concerning the use of GPU hardware.

### experiment ↠ config ↠ env

This section contains a dictionary of environmnet variables and their values.  Prior to the experiment being initiated by the runner the environment table will be loaded.  The envrionment table is current used for AWS authentication for S3 access and so this section should contain as a minimum the AWS_DEFAULT_REGION, AWS_ACCESS_KEY_ID, and AWS_SECRET_ACCESS_KEY variables.  In the future the AWS credentials for the artifacts will be obtained from the artifact block.

### experiment ↠ config ↠ cloud ↠ queue ↠ rmq

This variable will contain the rabbitMQ URI and configuration parameters if rabbitMQ was used by the system to queue this work.  The runner will ignore this value if it is passed through as it gets its queue information from the runner configuration store.

Copyright &copy 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
