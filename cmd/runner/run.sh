#!/bin/bash -e

export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:/usr/local/cuda/lib64
echo "**/usr/local/cuda"
ls /usr/local/cuda
echo "**/usr/local/cuda/lib64"
ls /usr/local/cuda/lib64
find / -name libnvidia-ml.so
nvidia-smi
SLACK_HOOK="https://hooks.slack.com/services/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" SLACK_ROOM="#studioml-ops" LOGXI_FORMAT=happy,maxcol=1024 LOGXI=*=TRC  ./runner -debug -sqs-certs certs -google-certs certs

