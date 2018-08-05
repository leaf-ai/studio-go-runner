FROM ubuntu:16.04

MAINTAINER karlmutch@gmail.com

ENV LANG C.UTF-8

ENV CUDA_DEB "https://developer.nvidia.com/compute/cuda/8.0/Prod2/local_installers/cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb"
ENV CUDA_PACKAGE_VERSION 8-0
ENV CUDA_FILESYS_VERSION 8.0
ENV NVIDIA_VERSION 384

RUN apt-get -y update

RUN \
    apt-get -y install software-properties-common wget openssl ssh curl jq apt-utils && \
    apt-get -y install make git gcc && apt-get clean

RUN cd /tmp && \
    wget --quiet -O /tmp/cuda.deb ${CUDA_DEB} && \
    dpkg -i /tmp/cuda.deb && \
    apt-get -y update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install --no-install-recommends nvidia-cuda-dev cuda-nvml-dev-${CUDA_PACKAGE_VERSION} && \
    rm /tmp/cuda.deb

RUN \
    ln -s /usr/local/cuda-${CUDA_FILESYS_VERSION} /usr/local/cuda && \
    ln -s /usr/local/cuda/targets/x86_64-linux/include /usr/local/cuda/include && \
    ln -s /usr/lib/nvidia-${NVIDIA_VERSION} /usr/lib/nvidia && \
    apt-get clean && \
    apt-get autoremove

ARG USER
ENV USER ${USER}
ARG USER_ID
ENV USER_ID ${USER_ID}
ARG USER_GROUP_ID
ENV USER_GROUP_ID ${USER_GROUP_ID}

RUN groupadd -f -g ${USER_GROUP_ID} ${USER} && \
    useradd -g ${USER_GROUP_ID} -u ${USER_ID} -ms /bin/bash ${USER}

USER ${USER}
WORKDIR /home/${USER}

ENV GO_VERSION 1.10.3

RUN cd /home/${USER} && \
    mkdir -p /home/${USER}/go && \
    wget -O /tmp/go.tgz https://storage.googleapis.com/golang/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar xzf /tmp/go.tgz && \
    rm /tmp/go.tgz

RUN wget -O /home/${USER}/go/bin/minio https://dl.minio.io/server/minio/release/linux-amd64/minio && \
    chmod +x /home/${USER}/go/bin/minio

ENV GOPATH=/project
ENV PATH=$GOPATH/bin:$PATH
ENV PATH=$PATH:/home/${USER}/go/bin
ENV GOROOT=/home/${USER}/go

VOLUME /project
WORKDIR /project/src/github.com/SentientTechnologies/studio-go-runner

# Done last to prevent lots of disruption when bumping versions
LABEL vendor="Sentient Technologies INC" \
      ai.sentient.module.version={{.duat.version}} \
      ai.sentient.module.name={{.duat.module}}

CMD /bin/bash -c 'go get github.com/karlmutch/duat && export LOGXI="*=DBG" && go run build.go . && go run build.go -r cmd'
