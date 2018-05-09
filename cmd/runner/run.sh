#!/bin/bash -e

export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:/usr/local/cuda/lib64:/usr/local/nvidia/lib64
nvidia-smi
nvidia-smi -L
echo "** /usr/local"
ls /usr/local/
echo "** /usr/lib"
ls /usr/lib/
echo "** /usr/local/nvidia/bin"
ls /usr/local/nvidia/bin
echo "** /usr/local/nvidia/lib64"
ls /usr/local/nvidia/lib64
echo "** /etc/ld.so.conf.d/cuda-8-0.conf"
cat /etc/ld.so.conf.d/cuda-8-0.conf
echo "** /usr/local/cuda-8.0/targets/x86_64-linux/lib"
ls /usr/local/cuda-8.0/targets/x86_64-linux/lib
find . -print
find / -name libnvidia-ml\* -print
find / -name nvidia-smi -print
SLACK_HOOK="https://hooks.slack.com/services/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" SLACK_ROOM="#studioml-ops" LOGXI_FORMAT=happy,maxcol=1024 LOGXI=*=DBG  /runner/runner-linux-amd64 -debug -sqs-certs certs/aws-sqs -sqs-prefix sqs_ms_ -amqp-url amqp://client:trueblue@40.117.135.66:5672/
