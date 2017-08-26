FROM ubuntu:16.04

MAINTAINER karlmutch@gmail.com

LABEL vendor="Sentient Technologies INC" \
      ai.sentient.version=0.0.0 \
      ai.sentient.module=studio-go-runner

ENV LANG C.UTF-8

ARG USER
ENV USER ${USER}
ARG USER_ID
ENV USER_ID ${USER_ID}
ARG USER_GROUP_ID
ENV USER_GROUP_ID ${USER_GROUP_ID}

ENV GO_VERSION 1.9
ENV CUDA_DEB "https://developer.nvidia.com/compute/cuda/8.0/Prod2/local_installers/cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb"
ENV CUDA_PACKAGE_VERSION 8-0
ENV CUDA_FILESYS_VERSION 8.0


RUN \
    apt-get -y update && \
    apt-get -y install software-properties-common wget openssl ssh curl jq apt-utils && \
    wget --quiet -O /tmp/cuda.deb ${CUDA_DEB} && \
    wget --quiet -O /tmp/go.tgz https://storage.googleapis.com/golang/go${GO_VERSION}.linux-amd64.tar.gz


RUN cd /tmp && \
    dpkg -i /tmp/cuda.deb && \
    apt-get -y update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install cuda cuda-toolkit-${CUDA_PACKAGE_VERSION} cuda-nvml-dev-${CUDA_PACKAGE_VERSION} && \
    ln -s /usr/local/cuda-${CUDA_FILESYS_VERSION} /usr/local/cuda && \
    apt-get -y install make git gcc && apt-get clean

RUN echo ${USER}
RUN groupadd -f -g ${USER_GROUP_ID} ${USER}
RUN useradd -g ${USER_GROUP_ID} -u ${USER_ID} -ms /bin/bash ${USER}

USER ${USER}
WORKDIR /home/${USER}

RUN cd /home/${USER} && \
    mkdir -p /home/${USER}/go && \
    tar xzf /tmp/go.tgz

ENV PATH=$PATH:/home/${USER}/go/bin
ENV GOROOT=/home/${USER}/go
ENV GOPATH=/project

VOLUME /project
WORKDIR /project/src/github.com/SentientTechnologies/studio-go-runner

CMD go build cmd/runner/*.go

#FROM ubuntu:16.04

#WORKDIR /root/
#COPY --from=builder /project/bin/. .
#CMD ["ls"]
