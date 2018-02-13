FROM nvidia/cuda:9.0-runtime-ubuntu16.04

RUN apt-get -y update && \
    apt-get -y install software-properties-common wget openssl ssh curl jq apt-utils

# Done last to prevent lots of disruption when bumping versions
LABEL vendor="Sentient Technologies INC" \
      ai.sentient.module.version=<repo-version></repo-version> \
      ai.sentient.module.name=studio-go-runner

COPY run.sh .
COPY bin/runner .

CMD /bin/bash -C ./run.sh
