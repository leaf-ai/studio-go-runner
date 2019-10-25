#!/bin/bash
mkdir -p installer/azure/minio
mkdir -p installer/azure/rabbitmq
wget -O installer/azure/minio/README.md https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/azure/minio/README.md
wget -O installer/azure/minio/user-data https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/azure/minio/user-data
wget -O installer/azure/rabbitmq/README.md https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/azure/rabbitmq/README.md
wget -O installer/azure/rabbitmq/user-data https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/azure/rabbitmq/user-data
echo "n" | ssh-keygen -t ed25519 -N "" -f ~/.ssh/id_ed25519
export PUBLIC_KEY=`cat ~/.ssh/id_ed25519.pub`
storage_account=`echo -e "bootlogs$(uuidgen | md5sum - | cut -c1-8)"`
#
envsubst < installer/azure/minio/user-data > minio-user-data-$storage_account
az login --use-device-code
az group create --name minio --location $LOCATION -o none
az storage account create --resource-group minio --name $storage_account  --sku standard_lrs -o none
az vm create --name minio --resource-group minio --location $LOCATION --data-disk-sizes-gb 10 --boot-diagnostics-storage $storage_account --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address-allocation static --size Standard_D4s_v3 --custom-data minio-user-data-$storage_account -o none
export MINIO_ADDRESS=$(az network public-ip list --resource-group minio --query "[].ipAddress" --output tsv)
az vm open-port --port 9000 --resource-group minio --name minio -o none
#
envsubst < installer/azure/rabbitmq/user-data > rabbitmq-user-data-$storage_account
az group create --name rabbitMQ --location $LOCATION
az storage account create --resource-group rabbitMQ --name $storage_account  --sku standard_lrs
az vm create --name rabbitMQ --resource-group rabbitMQ --location eastus --os-disk-size-gb 128 --boot-diagnostics-storage $storage_account --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address-allocation static --size Standard_D4s_v3 --custom-data rabbitmq-user-data-$storage_account -o none
export RMQ_ADDRESS=$(az network public-ip list --resource-group rabbitMQ --query "[].ipAddress" --output tsv)
az vm open-port --port 15672 --resource-group rabbitMQ --name rabbitMQ --priority 500
az vm open-port --port 5672 --resource-group rabbitMQ --name rabbitMQ --priority 501
echo "RabbitMQ IP Address" $RMQ_ADDRESS "Minio server IP Address" $MINIO_ADDRESS
