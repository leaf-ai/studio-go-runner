FROM ubuntu:16.04

RUN apt-get update && \
    apt-get install -y locales && \
    apt-get install -y language-pack-en && \
    update-locale "en_US.UTF-8" && \
    apt-get install -y --no-install-recommends ca-certificates apt-transport-https gnupg-curl && \
    apt-get install -y  --no-install-recommends python python-pip python3 python3-pip python3-dev python-dev git lshw && \
    NVIDIA_GPGKEY_SUM=d1be581509378368edeec8c1eb2958702feedf3bc3d17011adbf24efacce4ab5 && \
    NVIDIA_GPGKEY_FPR=ae09fe4bbd223a84b2ccfce3f60f4b3d7fa2af80 && \
    apt-key adv --fetch-keys https://developer.download.nvidia.com/compute/cuda/repos/ubuntu1604/x86_64/7fa2af80.pub && \
    apt-key adv --export --no-emit-version -a $NVIDIA_GPGKEY_FPR | tail -n +5 > cudasign.pub && \
    echo "$NVIDIA_GPGKEY_SUM  cudasign.pub" | sha256sum -c --strict - && rm cudasign.pub && \
    echo "deb https://developer.download.nvidia.com/compute/cuda/repos/ubuntu1604/x86_64 /" >> /etc/apt/sources.list.d/cuda.list && \
    echo "deb https://developer.download.nvidia.com/compute/machine-learning/repos/ubuntu1604/x86_64 /" >> /etc/apt/sources.list.d/nvidia-ml.list && \
    apt-get update && \
    apt-get -y upgrade


RUN apt-get install -y --no-install-recommends \
        cuda-nvrtc-8-0=8.0.61-1 \
        cuda-nvgraph-8-0=8.0.61-1 \
        cuda-cusolver-8-0=8.0.61-1 \
        cuda-cublas-8-0=8.0.61.2-1 \
        cuda-cufft-8-0=8.0.61-1 \
        cuda-curand-8-0=8.0.61-1 \
        cuda-cusparse-8-0=8.0.61-1 \
        cuda-npp-8-0=8.0.61-1 \
        cuda-cudart-8-0=8.0.61-1

RUN apt-get install -y --no-install-recommends \
        cuda-cudart-9-0=9.0.176-1 \
        cuda-command-line-tools-9-0 \
        cuda-cufft-9-0 \
        cuda-curand-9-0 \
        cuda-cusolver-9-0 \
        cuda-cusparse-9-0 \
        cuda-libraries-9-0=9.0.176-1 \
        cuda-cublas-9-0=9.0.176.3-1 \
        libnccl2=2.2.13-1+cuda9.0 && \
    apt-mark hold libnccl2

RUN echo "/usr/local/nvidia/lib" >> /etc/ld.so.conf.d/nvidia.conf && \
    echo "/usr/local/nvidia/lib64" >> /etc/ld.so.conf.d/nvidia.conf

ENV PATH /usr/local/nvidia/bin:/usr/local/cuda/bin:${PATH}
ENV LD_LIBRARY_PATH /usr/local/nvidia/lib:/usr/local/nvidia/lib64

# nvidia-container-runtime
ENV NVIDIA_VISIBLE_DEVICES all
ENV NVIDIA_DRIVER_CAPABILITIES compute,utility
ENV NVIDIA_REQUIRE_CUDA "cuda>=9.0"

# Pick up some TF dependencies
RUN apt-get install -y --no-install-recommends \
        build-essential \
        libcudnn5=5.1.10-1+cuda8.0 \
        libcudnn6=6.0.21-1+cuda8.0 \
        libcudnn7=7.1.4.18-1+cuda9.0 \
        libnccl2=2.2.13-1+cuda9.0 \
        libhdf5-serial-dev \
        libpng12-dev \
        libzmq3-dev \
        pkg-config \
        software-properties-common \
        unzip && \
    apt-get clean

RUN pip install --upgrade pip setuptools

#libssl-dev libcurl4-openssl-dev libsm6 libxrender-dev libxext-dev && \
RUN apt-get -y update && \
    apt-get -y install  && \
    pip install tensorflow-gpu==1.4.1 && \
    pip install tensorflow-gpu==1.8.0 && \
    pip install tensorflow-gpu==1.9.0 && \
    python -m pip install virtualenv==15.2.0 --force-reinstall && \
    python3 -m pip install virtualenv==15.2.0 --force-reinstall && \
    apt-get clean

RUN mkdir -p /runner/certs/aws-sqs
WORKDIR /runner

# Enable the ability to debug in-situ for python tasks
# RUN echo 0 | tee /proc/sys/kernel/yama/ptrace_scope

# Done last to prevent lots of disruption when bumping versions
LABEL vendor="Sentient Technologies INC" \
      ai.sentient.module.version=<repo-version></repo-version> \
      ai.sentient.module.name=studio-go-runner

# Add support for richer terminals to aid debugging etc
RUN mkdir -p /lib/terminfo/x && \
    mkdir -p /usr/local/share/terminfo/x
COPY add-ons/termite.terminfo /usr/local/share/terminfo/x/xterm-termite
COPY add-ons/termite.terminfo /lib/terminfo/x/xterm-termite

# Prometheus instrumented port
EXPOSE 9090

COPY run.sh /runner/run.sh
COPY bin/runner-linux-amd64 /runner/runner-linux-amd64
COPY bin/runner-linux-amd64-cpu /runner/runner-linux-amd64-cpu

CMD /bin/bash -C ./run.sh
