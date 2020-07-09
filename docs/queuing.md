Work Queuing
============

<!--ts-->

Table of Contents
=================

* [Table of Contents](#table-of-contents)
* [Motivation](#motivation)
* [Basic operation](#basic-operation)
* [Advanced topics](#advanced-topics)
  * [Reporting queues](#reporting-queues)
    * [Message format](#message-format)
<!--te-->
# Motivation

StudioML work is distributed using queues.  The order of work in any queue while services in a FIFO manner is not always strictly in order.

Any queue technology used by StudioML meets several basic requirements:

1. The queue has to requeue work if the runner looses contact with it automatically
2. Queues are at least once delivery

At this time the queue platforms used are AWS SQS and RabbitMQ.  Other implementations do exist but are not in wide use at this time.

# Basic operation

The runner is designed to allow projects within StudioML to have their experiments run across shared infrastructure in an equitable manner, at a machine level.

To do this the runner, when started, will query the credentials being used to access its supported queuing platforms to discover queues it can pull work from.

The runner will repeatedly scan queues looking for work that it can match with the GPU and CPU hardware it has available.  Once a message is recieved on a queue the resources it has requested are recorded and are used to avoid looking in the queue for work if insufficent resources are available.

GPU resources that are empty of any work will be associated with a queue when new work is scheduled until such time as the queue is drained at which point the GPU association will be dropped.

Should no GPU resources be available but there are CPU resources the runner will begin looking for queues that contain work that is CPU only and assign CPU resources to those queues.

Queued experiments that have been queried once are assumed to contain the same resource demands for all future experiments and the runner will assume this when selecting which queues to poll for work.

StudioML experimenters using the go runner can indicate that work in progress for a queue is to be aborted by deleting their queue/topics.

# Advanced topics

This section describes features that are an extension to standard StudioML implemented by the Go Runner.

## Reporting queues

In specific experiment failure cases the go runner will be unable to report results back to experimenters using the storage defined by experimenters.  For example in the event that the experiment message is not well formed, or the decryption of the message fails.  In most failure cases the failure itself can provide valuable information to the experiment.  In these cases reporting the failure using a response, or results queue is useful.  However there remain some cases where failures can result in a vector for an attack for example if a message is encrypted but has no valid signature which could be exploited for DDoS purposes and will not be sent.

In order to perform reporting of experiment progress, results, and failures a 'reporting queue' is used that is associated with the original queue across which experiment requests are being made.  The reporting queue name will use original queue, or topic name used for requests will have the string '\_reporting' appended.  Should the reporting queue exist then the go runner will post messages of interest about the state of the system in relation to requests or experiments on the request queue to this queue.

In the event that the queue is not used to receive requests the StudioML client is reponsible for abandoning the work.  This can be done withint RabbitMQ using a [Queue TTL](https://www.rabbitmq.com/ttl.html#queue-ttl), or in SQS using message retention, and visibility timeouts via the [SetQueueAttributes](https://docs.aws.amazon.com/AWSSimpleQueueService/latest/APIReference/API_SetQueueAttributes.html) function.  Queues or Topic are never created by the runner so the policies related to timeouts and time to live values are in the control of the StudioML client allowing experimenters to set appropriate values and control costs.

In the case of AWS SQS we suggest that because queues are not automatically deleted are an idle period that a tag is used to indicate when the queue is deemed to be of no use to the experimenter and that a schedule job in AWS be used to clear out queues based on the timestamp inside the SQS queue tag.

### Message format

Messages sent on the reporting queue are encoded as protobuf messages.  Detailed information about the message format can be found in [reports.proto](proto/reports.proto).


Copyright © 2019-2020 Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 license.
