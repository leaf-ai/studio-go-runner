FROM tensorflow/tensorflow:1.4.1-gpu
# FROM nvidia/cuda:9.0-cudnn7-runtime-ubuntu16.04

MAINTAINER Karl Mutch <karl.mutch@sentient.ai>

RUN apt-get -y update && \
    apt-get -y install software-properties-common wget openssl ssh curl jq apt-utils

# add tensorflow-gpu to use with gpu to sudo pip install
# to use on linux machines with gpus
RUN apt-get update && \
    apt-get -y install python-pip python-dev python3-pip python3-dev python3 git wget lshw && \
    python -m pip install --upgrade pip virtualenv && \
    python3 -m pip install --upgrade pip virtualenv

RUN apt-get install -y locales && apt-get install -y language-pack-en && update-locale "en_US.UTF-8"

#ENV DEBIAN_FRONTEND=noninteractive
#RUN apt-get install -y --no-install-recommends cuda-drivers==384.111

RUN mkdir -p /runner/certs/aws-sqs
WORKDIR /runner

# Done last to prevent lots of disruption when bumping versions
LABEL vendor="Sentient Technologies INC" \
      ai.sentient.module.version=<repo-version></repo-version> \
      ai.sentient.module.name=studio-go-runner

COPY run.sh /runner/.
COPY bin/runner /runner/.

CMD /bin/bash -C ./run.sh
