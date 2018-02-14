FROM tensorflow/tensorflow:1.5.0-gpu
MAINTAINER Karl Mutch <karl.mutch@sentient.ai>

RUN apt-get -y update && \
    apt-get -y install software-properties-common wget openssl ssh curl jq apt-utils

# Done last to prevent lots of disruption when bumping versions
LABEL vendor="Sentient Technologies INC" \
      ai.sentient.module.version=<repo-version></repo-version> \
      ai.sentient.module.name=studio-go-runner

# add tensorflow-gpu to use with gpu to sudo pip install
# to use on linux machines with gpus
RUN apt-get update && \
    apt-get -y install python-pip python-dev python3-pip python3-dev python3 git wget lshw && \
    python -m pip install --upgrade pip && \
    python3 -m pip install --upgrade pip

RUN mkdir /runner
WORKDIR /runner

COPY run.sh /runner/.
COPY bin/runner /runner/.

CMD /bin/bash -C ./run.sh
