# Metadata Introduction

The metadata features within the studioml go runner are designed to allow authors of python and containerized applications to sequester attributes of experiments into json files that are collected together with, or accompany results of experiments such as exported or saved models.

Experiment accessioning and management is a major requirement for both small and large teams in both research and commercial contexts.  Tasks run using studio go runner that generate conforming JSON output will have that output captured by the runner and stored as JSON blobs or files using the same storage endpoint as the experiment logs. Subsequently user workflows or downstream tools will be able to retreieve documents for any purpose for example, indexing and queries, model serving and monitoring (MLOps), or for ETL purposes.

# Experiment Metadata wrangling

## Storage organization

studioml runner when hosted tasks will monitor the console output of the task and will scrape any single line well formed JSON fragments.  These fragments are gathered every time a checkpoint of the task occurs and are used to build a JSON document that will be placed into the '\_metadata' artifact location on the storage endpoint specified by the experimenter initiating the studioml task.

The following figure shows the runtime layout of directories and files while an experiment is being run.  It shows the second pass at running an experiment after the first pass failed and as the second pass is running.

```
└── 4f9ba63a64ec0618.1
    ├── _metadata
    │   ├── scrape-host-awsdev-1gew0R.json
    │   └── scrape-host-awsdev-1gew1j.json
    ├── _metrics
    ├── modeldir
    ├── output
    │   └── output
    ├── tb
    └── workspace
        ├── experiment_template.json
        └── metadata-test.py
```

The \_metadata artifact shows files that have the file name composed from, the file type, the host key and host name, and an ID that can be sorted to reflect the time of creation.  These files allow the progress of the studioml task to be tracked across time and different machines within a studioml cluster.

studioml applications can retrieve these files from the storage platform choosen by the experiment and used to query experiment results using the raw console output, using JSON data emitted by the application as JSON documents in the case of the 'scrape-hist-xxx-tttttt.json' files.

If a bucket is used to store the experiments output data then the metadata artifacts will be uploaded as individual blobs, or files, allowing them to be selectively indexed or downloaded.  Their keys will appear as follows, given the previous example:

```
metadata/scrape-host-awsdev-1gew0R.json
metadata/scrape-host-awsdev-1gew1j.json
```

The metadata artifact is treated as a folder style artifact consisting of multiple individual files with three files per run and named using the pod/host name on which the run was located.  The following example show keys for artifacts from 2 attempted runs of an experiment, on hosts host-fe5917a, and host234c07a.

```
+ metadata
|
+--- scrape-host-fe5917a-1gKTNC.json
|
+--- scrape-host-234c07a-1gKTNw.json
```

Using individual objects, or files allows independent uploads of experiment activity enabling checksum based caching to be employed downstream and also to preserve atomic uploads for a host and experiment run combination.

The scrape files contain the metadata defined in the next subsection.

The trailing characters of the file names are significant in that they represent the timestamp at which the file was created in seconds since 1970 and then encoded using Base 62 format to create a chronology for file creation.  Refreshed files will retain their original names when updated.

Should annotations need injection into the scrape files by the experimenters application they must be added after the experiment has had its studioml.status tag updated to read 'completed'.  The application however must follow the rule that existing tags must not be modified.  It is envisioned that an application such as a session server or completion service (project orchestration) responsible for an entire project would wait for experiments to complete by querying the scrape files. Likewise ETL tasks populating a downstream ETL'ed database can stream scrape results into a downstream DB potentially adding a new tag on each extraction until the status is complete then doing a final extraction.

After completion the orchestrator can inspect the results in the scrape file and add information related to the entire experiment and the standing of each individual experiment in their respective scrape files.  Examples of the values orchestration might add could include model information such as a version number, or marking them as fit for deployment using application defined tags in the experiment section.

In some cases there may well be state the ML project orchestration software wishes to save for checkpointing and other purposes that are no part of the studioml scope.  In these cases the 3rd party software can store this independently of the studioml ecosystem possibly even on the same shared storage infrastructure.  However this is orthogonal to the runners and studioml.  Examples of this type of data might include project, cost center, and customer data.  Once each experiment is complete the orchestration can also add these tags to the finished experiment to assist with downstream ETL and queries that might need supporting.

If there is metadata that would be needed to reproduce the experiment then this should be added as an artifact to the input files for the experiment rather than waiting until the conclusion of the run to add it to the metadata artifact.

## JSON Document

JSON data scraped from the tasks console output will be captured and will be checked for being well-formed by the runner, validJSON on a single line.  The JSON data should be formatted as mergable fragments, or as JSON patch directives as defined by RFC6902, or RFC7386.  Examples of each appear below:

```
{"experiment": {"name": "testExpr", "max_run_length": 24, "current_run_position": 16}}
[{"op": "replace", "path": "/experiment/current_run_position", "value": 20}]
{"experiment": {"completed": "true"}}
[{"op": "remove", "path": "/experiment/current_run_position"}]
```

As an application progresses it can continue to emit merge fragments and patching directives updating the resulting document that the runner will checkpoint creating an upto the minute application state.

When the runner checkpoints a task, or when the task completes the JSON fragments these fragments are processed in the order they appeared to create a single JSON document and stored alongside the output log using a prefix of 'metadata/' as described in the previous subsection.

## runner JSON

JSON Data is also produced by the runner, when using python based workloads, detailing aspects of the runtime environment that can later be used by downstream tooling.

studioml data is gathered into a JSON map using studioml as the key.  User, or experiment data is by convention added using an experiment key.  For example the studioml generated pip dependency tree is placed into the JSON using the following as an example:

```
{
  "studioml": {
    "artifacts": {
      "_metadata": "s3://127.0.0.1:40130/bgnauro3p3itfkp5iuqg/_metadata.tar",
      "_metrics": "s3://127.0.0.1:40130/bgnauro3p3itfkp5iuqg/_metrics.tar",
      "modeldir": "s3://127.0.0.1:40130/bgnauro3p3itfkp5iuqg/modeldir.tar",
      "output": "s3://127.0.0.1:40130/bgnauro3p3itfkp5iuqg/output.tar",
      "tb": "s3://127.0.0.1:40130/bgnauro3p3itfkp5iuqg/tb.tar",
      "workspace": "s3://127.0.0.1:40130//bgnauro3p3itfkp5iuqg/workspace.tar"
    },
    "experiment": {
      "key": "e5e90feb-a6e5-4668-b885-c1789f74ad23",
      "project": "goldengun"
    },
    "pipdeptree": [
      {
        "dependencies": [],
        "package": {
          "installed_version": "3.1.0",
          "package_name": "setuptools-scm",
          "key": "setuptools-scm"
        }
      },
      {
        "dependencies": [],
        "package": {
          "installed_version": "1.24",
          "package_name": "urllib3",
          "key": "urllib3"
        }
      },
...
      }
    ]
  },
...
}
```
Application JSON output is added simply by sending JSON merge fragments, or JSON patch directives.  Should the application echo the following:

```
{"experiment": {"name": "dummy pass"}}
{"experiment": {"name": "Zaphod Beeblebrox"}}
```

the result would appear in the JSON file as:

```
{
  "experiment": {
    "name": "Zaphod Beeblebrox"
  },
  "studioml": {
  }
...
}
```

## JSON output for MLOps

MLOps metadata for StudioML tasks is managed via a JSON document. The description of the metadata JSON document generally has the ability to store any JSON.  In the case of portions of the MLOps document that have a studioml root for their JSON path there is defined a schema.  For the other portions the JSON stored is undefined and projects or organizations are free to inject their own fragments.

In order to retain the operational significant logs the JSON metadata is used to store an array of log entries, which the schema does define.  Logs are stored this way to enable indexing and it is recommended that strings added to the log are JSON and structured in such a way as to allow their use as wide events for observability purposes.

```
{
	"studioml": {
		"artifacts": {
			"_metadata": "s3://s3-us-west-2.amazonaws.com/karl-mutch-rmq/experiments/1624770769_663637bc-f768-403f-b005-0c331954f290/_metadata.tar",
			"_metrics": "s3://s3-us-west-2.amazonaws.com/karl-mutch-rmq/experiments/1624770769_663637bc-f768-403f-b005-0c331954f290/_metrics.tar",
			"modeldir": "s3://s3-us-west-2.amazonaws.com/karl-mutch-rmq/experiments/1624770769_663637bc-f768-403f-b005-0c331954f290/modeldir.tar",
			"output": "s3://s3-us-west-2.amazonaws.com/karl-mutch-rmq/experiments/1624770769_663637bc-f768-403f-b005-0c331954f290/output.tar",
			"retval": "s3://s3-us-west-2.amazonaws.com/karl-mutch-rmq/experiments/1624770769_663637bc-f768-403f-b005-0c331954f290/retval.tar",
			"tb": "s3://s3-us-west-2.amazonaws.com/karl-mutch-rmq/experiments/1624770769_663637bc-f768-403f-b005-0c331954f290/tb.tar",
			"workspace": "s3://s3-us-west-2.amazonaws.com/karl-mutch-rmq/blobstore/b77b9495db5bd3fecaedae83ba3da80e2908ef5c10bb255eb932fb1682b1960a.tar"
		},
		"experiment": {
			"key": "1624770769_663637bc-f768-403f-b005-0c331954f290",
			"project": "\u003cno value\u003e",
		},
		"host": "studioml-go-runner-23efa68b-c9cb-4f1a-82bc-550b6df4f9d5-k6nkk",
		"load_time": "2021-06-27T05:23:11.140789961+00:00",
		"log": [{
			"msg": "Start",
			"ts": "2021-06-27T05:25:54,013499823+00:00"
		}, {
			"msg": "",
			"ts": "0"
		}, {
			"msg": "Stop",
			"ts": "2021-06-27T05:26:26,636437415+00:00"
		}],
		"start_time": "2021-06-27T05:25:53.787879967+00:00",
		"stop_time": "2021-06-27T05:26:27.936453123+00:00"
	}
}
```

As previous discussed the presence of JSON editing allows experiments to progress adding individual log entries as items at the end of the JSON log array creating an ongoing record of the progress of the experiment.

The JSON editing is restricted in the studioml JSON root to add operations only.  Any other root can be changed in any fashion.  Using add ooperations only creates an audit trail of all activities.

Logging can be implemented through the use of the add operation with a timestamp using the command line as an example.  If you wish to implement the log insertion using python keep in mind the timestamp ISO8601 as per the linux date command format.  Looking at the ISON path shows the use of the - which has the effect of appending to the log array.

```
echo "[{\"op\": \"add\", \"path\": \"/studioml/log/-\", \"value\": {\"ts\": \"`date -u -Ins`\", \"msg\":\"Stop\"}}]"
```

If you wanted to insert log lines from within python the following code can be used:

```
import time
import datetime
import io

tStamp = datetime.datetime.utcnow().replace(tzinfo=datetime.timezone.utc).isoformat()

print('[{"op": "add", "path": "/studioml/log/-", "value": {"ts": "{0}", "msg:"Example log message"}}]', tStamp)
```

It is of course tedious if you have to decorate every log entry with a time stamp so as a convineance the go runner does have an option which will auto decorate an standard output and standard error messages with json directives for inclusion in the MLOps logs, --schema-logs.  Standard practice for using this option is to include it in your deployment configuration as an environment variable.

# Storage platforms and query capabilities

The json files when used within AWS implementations can be used as ingest points for hive queries using AWS Athena.

The following Hive DDL can be used to create and populate tables with experiment metadata:

```
CREATE DATABASE StudioML;
CREATE EXTERNAL TABLE IF NOT EXISTS `StudioML`.`metadata` (
	`studioml` STRUCT<
        `artifacts`:STRUCT<
        `_metadata`:STRING,
        `_metrics`:STRING,
        `modeldir`:STRING,
        `output`:STRING,
        `retval`:STRING,
        `tb`:STRING,
        `workspace`:STRING>,
    `experiment`:STRUCT<
        `key`:STRING,
        `author`:STRING,
        `project`:STRING,
        `project_version`:STRING,
        `project_experiment`:STRING>,
    `host`:STRING,
    `load_time`:STRING,
    `log`:ARRAY<STRUCT<
        `msg`:STRING,
        `ts`:STRING>>,
    `start_time`:STRING,
    `stop_time`:STRING>)
ROW FORMAT SERDE
	 'org.openx.data.jsonserde.JsonSerDe'
LOCATION
	 's3://karl-mutch-rmq/metadata/';
```

All studioml and experiment metadata can now be queried using Athena as shown in the following example walk-through:

The first step is to run after creating the Database and Table is to run a StudioML experiment.  When the experiment has been queued you will be shown an experiment ID which is a UUID.  This pre supposes that an EKS cluster has been created that perform the experiment.

```
$ cat ~/.studioml/config.yaml
database:
    type: s3
    endpoint: https://s3-us-west-2.amazonaws.com
    bucket: "karl-mutch-metadata"
    authentication: none
    credentials:
        aws:
            access_key: [Your AWS Access key]
            secret_access_key: [Your AWS Secret Access Key]

storage:
    type: s3
    endpoint: https://s3-us-west-2.amazonaws.com
    bucket: "karl-mutch-rmq"
    credentials:
        aws:
            access_key: [Your AWS Access key]
            secret_access_key: [Your AWS Secret Access Key]

verbose: debug
saveWorkspaceFrequency: 5m
experimentLifetime: 120m

resources_needed:
    cpus: 2
    gpus: 1
    hdd: 20gb
    ram: 32gb

$ studio run --lifetime=240m --max-duration=20m --gpuMem 8G --gpus 1 --queue=sqs_StudioML_kmutch --force-git train_mnist.py
...
2021-06-26 22:12:52 INFO   studio-runner - studio run: submitted experiment 1624770769_663637bc-f768-403f-b005-0c331954f290
2021-06-26 22:12:52 INFO   studio-runner - Added 1 experiment(s) in 1 seconds to queue sqs_StudioML_kmutch
```

```
SELECT studioml.log FROM karlmutchrmq.metadata WHERE studioml.experiment.key='1624770769_663637bc-f768-403f-b005-0c331954f290';
[{msg=Start, ts=2021-06-27T05:25:54,013499823+00:00}, {msg=, ts=0}, {msg=Stop, ts=2021-06-27T05:26:26,636437415+00:00}]
```

For more information please see, https://docs.aws.amazon.com/athena/latest/ug/work-with-data.html.

# LEAF UI and extensible MLOps lifecycle metadata

The demands of MLOps requires that high order, or metadata unrelated to the StudioML operations be also stored and made accessible to downstream tooling.

In order to do this additional S3 root keys are leveraged by the LEAF UI and LEAF servers, for example s3://karl-mutch-rmq/leaf.  The UI places additional json blobs into the bucket to represent LEAF level project and experiment metadata, this can be directly or via the mdserver.  The LEAF applications should define their own DDL schemas for this purpose.

The LEAF MLOps data and StudioML data do not provide referential integrity. Instead they are immutable and use S3 blob versions when referring to significantc documents.

# Downstream ETL and enterprise integration

When wrangling JSON documents the jq tool has proved invaluable, https://stedolan.github.io/jq/.

The design of the metadata artifact allows the creation of downstream applications that extract data from a studioml data store, such as S3, while experiments are in one of two states:

1. experiments in flight
2. experiments that have ceased active processing

Performing ETL on experiments that have ceased processing can be easily implemented via the ETL marking experiments as exported using custom tags in the experiment block.  Any experiments without the exported tag can then be selected using either a JSON query engine for simple iteration of scrape JSON files.  Using a query engine such as AWS Athena or Google datastore is another method employing S3 select on the JSON studioml structure and the status field with a value of completed.  If a query engine is not available the files store or blob heirarchy can be traversed and the most recent run scrapes selected using the last dash delimited portion of the file name as a sortable timestamp equivalent then marshalling the JSON to check on the status.

ETL processing if performed using a long lived daemon can track experiments still in progress using a membership test filter on in-memory data structure to exclude or include experiments for ETL, an example of this is in-memory cuckoo filter, https://brilliant.org/wiki/cuckoo-filter/, preventing unnessasary processing of JSON artifacts for experiments that have already completed, or which are no longer of interest.  If iteration is being used then the timestamp portion of the file name also be used to exclude JSON scrapes that are too old to be relevant.  For storage platforms that store access and modification file times there are also opportunities to avoid needless processing.

Copyright © 2019-2021 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
