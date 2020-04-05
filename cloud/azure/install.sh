#!/bin/bash
[ -z "$MINIO_ACCESS_KEY" ] && { echo "The MINIO_ACCESS_KEY, and MINIO_SECRET_KEY environment variables must be defined."; exit -1 ; }
[ -z "$MINIO_SECRET_KEY" ] && { echo "The MINIO_ACCESS_KEY, and MINIO_SECRET_KEY environment variables must be defined."; exit -1 ; }
[ -z "$RMQ_ADMIN_PASSWORD" ] && { echo "The RMQ_ADMIN_PASSWORD, and RMQ_USER_PASSWORD environment variables must be defined."; exit -1 ; }
[ -z "$RMQ_USER_PASSWORD" ] && { echo "The RMQ_ADMIN_PASSWORD, and RMQ_USER_PASSWORD environment variables must be defined."; exit -1 ; }

[ -z "$LOCATION" ] && { echo "The Azure location LOCATION environment variables must be defined."; exit -1 ; }

mkdir -p installer/azure/minio
mkdir -p installer/azure/rabbitmq
wget -O installer/azure/minio/README.md https://raw.githubusercontent.com/leaf-ai/studio-go-runner/master/cloud/azure/minio/README.md
wget -O installer/azure/minio/user-data https://raw.githubusercontent.com/leaf-ai/studio-go-runner/master/cloud/azure/minio/user-data
wget -O installer/azure/rabbitmq/README.md https://raw.githubusercontent.com/leaf-ai/studio-go-runner/master/cloud/azure/rabbitmq/README.md
wget -O installer/azure/rabbitmq/user-data https://raw.githubusercontent.com/leaf-ai/studio-go-runner/master/cloud/azure/rabbitmq/user-data
echo "n" | ssh-keygen -t ed25519 -N "" -f ~/.ssh/id_ed25519
export PUBLIC_KEY=`cat ~/.ssh/id_ed25519.pub`
storage_account=`echo -e "bootlogs$(uuidgen | md5sum - | cut -c1-8)"`
#
[ -z "$minio_resource_group" ] && minio_resource_group=minio
envsubst < installer/azure/minio/user-data > minio-user-data-$storage_account
az login --use-device-code
az group create --name $minio_resource_group --location $LOCATION -o none
az storage account create --resource-group $minio_resource_group --name $storage_account  --sku standard_lrs -o none
az vm create --name minio --resource-group $minio_resource_group --location $LOCATION --data-disk-sizes-gb 10 --boot-diagnostics-storage $storage_account --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address "" --size Standard_D4s_v3 --custom-data minio-user-data-$storage_account -o none
export MINIO_ADDRESS=$(az network public-ip list --resource-group $minio_resource_group --query "[].ipAddress" --output tsv)
az vm open-port --port 9000 --resource-group $minio_resource_group --name minio -o none
az network nsg rule delete --resource-group $minio_resource_group --nsg-name minioNSG -n default-allow-ssh
#
[ -z "$rmq_resource_group" ] && rmq_resource_group=rabbitMQ
envsubst < installer/azure/rabbitmq/user-data > rabbitmq-user-data-$storage_account
az group create --name $rmq_resource_group --location $LOCATION
az storage account create --resource-group $rmq_resource_group --name $storage_account  --sku standard_lrs
az vm create --name rabbitMQ --resource-group $rmq_resource_group --location eastus --os-disk-size-gb 128 --boot-diagnostics-storage $storage_account --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address "" --size Standard_D4s_v3 --custom-data rabbitmq-user-data-$storage_account -o none
export RMQ_ADDRESS=$(az network public-ip list --resource-group $rmq_resource_group --query "[].ipAddress" --output tsv)
az vm open-port --port 15672 --resource-group $rmq_resource_group --name rabbitMQ --priority 500
az vm open-port --port 5672 --resource-group $rmq_resource_group --name rabbitMQ --priority 501
az network nsg rule delete --resource-group $rmq_resource_group --nsg-name rabbitMQNSG -n default-allow-ssh
echo "RabbitMQ IP Address" $RMQ_ADDRESS "Minio server IP Address" $MINIO_ADDRESS
