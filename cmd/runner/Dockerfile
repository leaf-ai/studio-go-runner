FROM tensorflow/tensorflow:1.4.1-gpu
# FROM nvidia/cuda:9.0-cudnn7-runtime-ubuntu16.04

MAINTAINER Karl Mutch <karl.mutch@sentient.ai>

RUN apt-get clean && \
    rm /etc/apt/sources.list.d/cuda.list && \
    echo "***" && \
    rm /etc/apt/sources.list.d/nvidia-ml.list && \
    apt-get -y update && \
    apt-get -y install software-properties-common wget openssl ssh curl jq apt-utils dnsutils vim htop

# add tensorflow-gpu to use with gpu to sudo pip install
# to use on linux machines with gpus
RUN apt-get -y update && \
    apt-get -y upgrade

RUN apt-get -y install python-pip python-dev python3-pip python3-dev python3 git wget lshw && \
    apt-get -y install libssl-dev libcurl4-openssl-dev libsm6 libxrender-dev libxext-dev && \
    pip3 install --upgrade pip==9.0.3 --force-reinstall && \
    pip install --upgrade pip==9.0.3 --force-reinstall && \
    python -m pip install pip==9.0.3 virtualenv==15.2.0 --force-reinstall && \
    python3 -m pip install pip==9.0.3 virtualenv==15.2.0 --force-reinstall && \
    pip install --upgrade Python==2.7.12

RUN apt-get install -y locales && \
    apt-get install -y language-pack-en && \
    update-locale "en_US.UTF-8"

RUN mkdir -p /runner/certs/aws-sqs
WORKDIR /runner

# Done last to prevent lots of disruption when bumping versions
LABEL vendor="Sentient Technologies INC" \
      ai.sentient.module.version=<repo-version></repo-version> \
      ai.sentient.module.name=studio-go-runner

# Add support for richer terminals to aid debugging etc
RUN mkdir -p /lib/terminfo/x
RUN mkdir -p /usr/local/share/terminfo/x
COPY add-ons/termite.terminfo /usr/local/share/terminfo/x/xterm-termite
COPY add-ons/termite.terminfo /lib/terminfo/x/xterm-termite

# Prometheus instrumented port
EXPOSE 9090

COPY run.sh /runner/.
COPY bin/runner-linux-amd64 /runner/.
COPY bin/runner-linux-amd64-cpu /runner/.

CMD /bin/bash -C ./run.sh
