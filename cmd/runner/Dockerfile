FROM tensorflow/tensorflow:1.4.1-gpu
# FROM nvidia/cuda:9.0-cudnn7-runtime-ubuntu16.04

MAINTAINER Karl Mutch <karl.mutch@sentient.ai>

RUN apt-get clean && \
    rm /etc/apt/sources.list.d/cuda.list && \
    echo "***" && \
    rm /etc/apt/sources.list.d/nvidia-ml.list && \
    apt-get -y update && \
    apt-get -y install software-properties-common wget openssl ssh curl jq apt-utils

# add tensorflow-gpu to use with gpu to sudo pip install
# to use on linux machines with gpus
RUN apt-get -y update && \
    apt-get -y install python-pip python-dev python3-pip python3-dev python3 git wget lshw && \
    pip3 install --upgrade pip==9.0.3 --force-reinstall && \
    pip install --upgrade pip==9.0.3 --force-reinstall && \
    python -m pip install pip==9.0.3 virtualenv==15.2.0 --force-reinstall && \
    python3 -m pip install pip==9.0.3 virtualenv==15.2.0 --force-reinstall

RUN apt-get install -y locales && apt-get install -y language-pack-en && update-locale "en_US.UTF-8"

#RUN apt-get install -y --no-install-recommends cuda-drivers==384.111
#
#RUN apt-get install -y dkms && \
#    add-apt-repository ppa:graphics-drivers/ppa && \
#    apt-get update && \
#    apt-get install -y nvidia-384
#
#RUN wget -q https://developer.nvidia.com/compute/cuda/8.0/Prod2/local_installers/cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb && \
    #dpkg -i cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb && \
    #rm cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb && \
    #wget -q https://developer.nvidia.com/compute/cuda/8.0/Prod2/patches/2/cuda-repo-ubuntu1604-8-0-local-cublas-performance-update_8.0.61-1_amd64-deb && \
    #dpkg -i cuda-repo-ubuntu1604-8-0-local-cublas-performance-update_8.0.61-1_amd64-deb && \
    #rm cuda-repo-ubuntu1604-8-0-local-cublas-performance-update_8.0.61-1_amd64-deb && \
    #apt-key add /var/cuda-repo-8-0-local-ga2/7fa2af80.pub && \
    #apt-get update

#ENV DEBIAN_FRONTEND=noninteractive
#RUN apt-get install -y --no-install-recommends cuda-nvml-dev-8-0
#RUN apt-get install -y --no-install-recommends cuda-command-line-tools-8-0
RUN mkdir -p /runner/certs/aws-sqs
WORKDIR /runner

# Done last to prevent lots of disruption when bumping versions
LABEL vendor="Sentient Technologies INC" \
      ai.sentient.module.version=<repo-version></repo-version> \
      ai.sentient.module.name=studio-go-runner

COPY run.sh /runner/.
COPY bin/runner-linux-amd64 /runner/.
COPY bin/runner-linux-amd64-cpu /runner/.

CMD /bin/bash -C ./run.sh
