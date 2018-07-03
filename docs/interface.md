# Interfacing and Integration

This document describes the interfaces, and interchange format between the StudioML frontend and runners that process StudioML workload.

## Introduction

StudioML has two major modules.

The front end that shepherds experiments on behalf of users and packages them up to be queued to a backend runner

The runner that receieves json formatted messages from the StudioML front end across a message queue

There are other tools that StudioML offers for reporting and management of experiment artifacts that are not within the scope of this document.

## Runners

This project implements a StudioML runner, however it is not specific to StudioML.  This runner could be used to deliver and execute and python code within a virtualenv that the runner supplies.

Any standard runners can accept a standalone virtualenv with no associated container.  The go runner, this present project, has been extended to allow clients to also send work that has a Singularity container specified.

In the first case, virtualenv only, the runner implcitly trusts that any work received is trusted and is not malicous.  In this mode the runner makes not attempt to protect the integrity of the host it is deployed into.

In the second case if a container is specified it will be used to launch work and the runner will rely upon the container runtime to prevent leakage into the host.

## Queuing

The StudioML eco system relies upon a message queue to buffer work being sent by the StudioML client to any arbitrary runner that is subscribed to the experimenters choosen queuing service.  StudioML support multiple queuing technologies including, AWS SQS, Google PubSub, local file system, and RabbitMQ.  The reference implementation is RabbitMQ for the purposes of this present project.

Additional queuing technologies can be added if desired to the StudioML (https://github.com/studioml/studio.git), and go runner (https://github.com/SentientTechnologies/studio-go-runner.git) code bases and a pull request submitted.

When using a queue the StudioML eco system relies upon a reliable, at-least-once, messaging system.  An additional requirement for queuing systems is that if the worker disappears, or work is not reclaimed by the worker as progress is made that the work is requeued by the broker automatically.

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
    "pythonver": 2,
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
        "rmq": "amqp://user:password@10.230.72.19:5672/"
      }
    }
  }
}
```

### experiment ↠ pythonver

The value for this tag must be an integer 2 or 3 for the specific python version requested by the experimenter.

### experiment ↠ args

A list of the command line arguments to be supplied to the python interpreter that will be passed into the main of the running python job.


### experiment ↠ max_duration

The period of time that and experiment is permitted to run in a single attempt.

### experiment ↠ filename

### experiment ↠ project

### experiment ↠ artifacts

type

workspace, output

### experiment ↠ artifacts ↠ [type] ↠ bucket

### experiment ↠ artifacts ↠ [type] ↠ qualified

### experiment ↠ artifacts ↠ [type] ↠ key

### experiment ↠ artifacts ↠ [type] ↠ mutable

### experiment ↠ artifacts ↠ [type] ↠ unpack

### experiment ↠ artifacts ↠ resources_needed

### experiment ↠ artifacts ↠ pythonenv

### experiment ↠ config
### experiment ↠ config ↠ experimentLifetime

### experiment ↠ verbose

### experiment ↠ saveWorkspaceFrequency

### experiment ↠ database
### experiment ↠ database ↠ type
### experiment ↠ database ↠ authentication
### experiment ↠ database ↠ endpoint
### experiment ↠ database ↠ bucket

### experiment ↠ storage ↠ type
### experiment ↠ storage ↠ endpoint
### experiment ↠ storage ↠ bucket

### experiment ↠ server ↠ authentication

### experiment ↠ resources_needed

### experiment ↠ env

### experiment ↠ cloud ↠ queue ↠ rmq
