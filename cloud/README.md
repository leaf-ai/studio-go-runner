# Introduction

The cloud sub-directory contains scripts that can be used within a cloud context to install the queue and storage server dependencies needed for running the studio-go-runner.

# Infrastructure Installation

in order to make use of these scripts four environment variables should be defined first for the Minio and the RabbitMQ access secrets, or passwords:

MINIO_ACCESS_KEY
MINIO_SECRET_KEY
RMQ_ADMIN_PASSWORD
RMQ_USER_PASSWORD


Use the following commands to download the installation script.  Before running it you must change the file to include password and AWS, or Azure location information:
 
```shell
export CLOUD_VENDOR=aws  # azure is also a valid value
wget -O install_custom.sh https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/$CLOUD_VENDOR/install.sh
```

Using vim for another file editing tool on linux to change the script to your needs, then run the script and use the echo command to give you the IP addresses of the server that were installed.

```shell
./install_custom.sh
```
