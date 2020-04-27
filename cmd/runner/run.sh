#!/bin/bash -e

# Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

echo "pip 3 freeze and config"
pip3 freeze
pip3 config list
pip3 -V

export LD_LIBRARY_PATH=${LD_LIBRARY_PATH}:/usr/local/cuda/lib64:/usr/local/nvidia/lib64
echo "env"
env
echo "export"
export
sum /runner/*
echo "** /usr/local"
ls /usr/local/
echo "** /usr/lib"
ls /usr/lib/
if [ -n "$CACHE_DIR" ]; then
    mkdir $CACHE_DIR
fi
if [ ! -d /proc/driver/nvidia/gpus ] || [ ! "$(ls -A /proc/driver/nvidia/gpus)" ]
then
    /runner/runner-linux-amd64-cpu
else
    nvidia-smi
    nvidia-smi -L
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
    /runner/runner-linux-amd64
fi
