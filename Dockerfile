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

ENV GO_VERSION 1.9.1
ENV CUDA_DEB "https://developer.nvidia.com/compute/cuda/8.0/Prod2/local_installers/cuda-repo-ubuntu1604-8-0-local-ga2_8.0.61-1_amd64-deb"
ENV CUDA_PACKAGE_VERSION 8-0
ENV CUDA_FILESYS_VERSION 8.0

RUN apt-get -y update


RUN \
    apt-get -y install software-properties-common wget openssl ssh curl jq apt-utils && \
    apt-get -y install make git gcc && apt-get clean

RUN cd /tmp && \
    wget --quiet -O /tmp/cuda.deb ${CUDA_DEB} && \
    dpkg -i /tmp/cuda.deb && \
    apt-get -y update && \
    DEBIAN_FRONTEND=noninteractive apt-get -y install cuda cuda-toolkit-${CUDA_PACKAGE_VERSION} cuda-nvml-dev-${CUDA_PACKAGE_VERSION} && \
    ln -s /usr/local/cuda-${CUDA_FILESYS_VERSION} /usr/local/cuda && \
    rm /tmp/cuda.deb

RUN \
    apt-get clean && \
    groupadd -f -g ${USER_GROUP_ID} ${USER} && \
    useradd -g ${USER_GROUP_ID} -u ${USER_ID} -ms /bin/bash ${USER}

USER ${USER}
WORKDIR /home/${USER}

RUN cd /home/${USER} && \
    mkdir -p /home/${USER}/go && \
    wget -O /tmp/go.tgz https://storage.googleapis.com/golang/go${GO_VERSION}.linux-amd64.tar.gz && \
    tar xzf /tmp/go.tgz && \
    rm /tmp/go/tgz && \
    wget -O /home/${USER}/go/bin/jfrog "https://bintray.com/jfrog/jfrog-cli-go/download_file?file_path=1.11.2%2Fjfrog-cli-linux-386%2Fjfrog" && \
    chmod +x /home/${USER}/go/bin/jfrog


ENV PATH=$PATH:/home/${USER}/go/bin
ENV GOROOT=/home/${USER}/go
ENV GOPATH=/project

VOLUME /project
WORKDIR /project/src/github.com/SentientTechnologies/studio-go-runner

CMD /bin/bash -C ./build.sh

#FROM ubuntu:16.04

#WORKDIR /root/
#COPY --from=builder /project/bin/. .
#CMD ["ls"]
