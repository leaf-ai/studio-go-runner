#!/bin/bash -e

export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:/usr/local/cuda/lib64
nvidia-smi
find . -print
SLACK_HOOK="https://hooks.slack.com/services/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" SLACK_ROOM="#studioml-ops" LOGXI_FORMAT=happy,maxcol=1024 LOGXI=*=TRC  ./runner -debug -sqs-certs certs/aws-sqs -google-certs certs/google

