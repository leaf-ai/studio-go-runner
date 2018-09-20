FROM ubuntu:16.04

MAINTAINER karlmutch@gmail.com

ENV LANG C.UTF-8

ENV CUDA_8_DEB "https://developer.nvidia.com/compute/cuda/8.0/Prod2/local_installers/cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb"
ENV CUDA_9_DEB "https://developer.nvidia.com/compute/cuda/9.0/Prod/local_installers/cuda-repo-ubuntu1604-9-0-local_9.0.176-1_amd64-deb"
ENV CUDA_PACKAGE_VERSION 8-0
ENV CUDA_FILESYS_VERSION 8.0
ENV NVIDIA_VERSION 384

RUN apt-get -y update

RUN \
    apt-get -y install software-properties-common wget openssl ssh curl jq apt-utils && \
    apt-get -y install make git gcc && apt-get clean

RUN cd /tmp && \
    wget -q -O /tmp/cuda_8.deb ${CUDA_8_DEB} && \
    dpkg -i /tmp/cuda_8.deb && \
    apt-get -y update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install --no-install-recommends nvidia-cuda-dev cuda-nvml-dev-${CUDA_PACKAGE_VERSION} && \
    rm /tmp/cuda*.deb && \
    apt-get clean

    #wget --quiet -O /tmp/cuda_9.deb ${CUDA_9_DEB} && \
    #dpkg -i /tmp/cuda_9.deb && \
    #    apt-key add /var/cuda-repo-9-0-local/7fa2af80.pub && \
    #apt-get -y update && \
    #DEBIAN_FRONTEND=noninteractive apt-get -y install --no-install-recommends cuda-runtime-9-2 && \
    #rm /tmp/cuda*.deb

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

ENV GO_VERSION 1.11

ENV GOPATH=/project
ENV PATH=$GOPATH/bin:$PATH
ENV PATH=$PATH:/home/${USER}/.local/bin:/home/${USER}/go/bin
ENV GOROOT=/home/${USER}/go

RUN cd /home/${USER} && \
    mkdir -p /home/${USER}/go && \
    wget -q -O /tmp/go.tgz https://storage.googleapis.com/golang/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar xzf /tmp/go.tgz && \
    rm /tmp/go.tgz

RUN mkdir -p /home/${USER}/.local/bin && \
    wget -q -O /home/${USER}/.local/bin/minio https://dl.minio.io/server/minio/release/linux-amd64/minio && \
    chmod +x /home/${USER}/.local/bin/minio

VOLUME /project
WORKDIR /project/src/github.com/SentientTechnologies/studio-go-runner

CMD /bin/bash -c 'go get github.com/karlmutch/duat && go get github.com/karlmutch/enumer && dep ensure && go generate ./internal/types && go run -tags NO_CUDA build.go -r -dirs=internal && go run -tags NO_CUDA build.go -r -dirs=cmd'

# Done last to prevent lots of disruption when bumping versions
LABEL vendor="Sentient Technologies INC" \
      ai.sentient.module.version={{.duat.version}} \
      ai.sentient.module.name={{.duat.module}}

