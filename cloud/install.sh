mkdir -p installer/minio
mkdir -p installer/rabbitmq
wget -O installer/minio/README.md https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/minio/README.md
wget -O installer/minio/user-data https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/minio/user-data
wget -O installer/rabbitmq/README.md https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/rabbitmq/README.md
wget -O installer/rabbitmq/user-data https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/rabbitmq/user-data
echo "n" | ssh-keygen -t ed25519 -N "" -f ~/.ssh/id_ed25519
export PUBLIC_KEY=`cat ~/.ssh/id_ed25519.pub`
storage_account=`echo -e "bootlogs$(uuidgen | md5sum - | cut -c1-8)"`
export LOCATION=eastus
export MINIO_ACCESS_KEY=[An access key you choose and is secret to you, and users of StudioML, LEAF]
export MINIO_SECRET_KEY=[A secret key you choose and is secret to you, and users of StudioML, LEAF.  Must be at least 8 characters in length.]
envsubst < installer/minio/user-data > minio-user-data-$storage_account
az login --use-device-code
az group create --name minio --location $LOCATION -o none
az storage account create --resource-group minio --name $storage_account  --sku standard_lrs -o none
az vm create --name minio --resource-group minio --location $LOCATION --data-disk-sizes-gb 10 --boot-diagnostics-storage $storage_account --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address-allocation static --size Standard_D4s_v3 --custom-data minio-user-data-$storage_account -o none
export MINIO_ADDRESS=$(az network public-ip list --resource-group minio --query "[].ipAddress" --output tsv)
az vm open-port --port 9000 --resource-group minio --name minio -o none
export RMQ_ADMIN_PASSWORD=[A secret key you choose and is secret to the administrator]
export RMQ_USER_PASSWORD=[A secret key you choose and is secret to users of StudioML, or LEAF]
envsubst < installer/rabbitmq/user-data > rabbitmq-user-data-$storage_account
az group create --name rabbitMQ --location $LOCATION
az storage account create --resource-group rabbitMQ --name $storage_account  --sku standard_lrs
az vm create --name rabbitMQ --resource-group rabbitMQ --location eastus --os-disk-size-gb 128 --boot-diagnostics-storage $storage_account --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address-allocation static --size Standard_D4s_v3 --custom-data rabbitmq-user-data-$storage_account -o none
export RMQ_ADDRESS=$(az network public-ip list --resource-group rabbitMQ --query "[].ipAddress" --output tsv)
az vm open-port --port 15672 --resource-group rabbitMQ --name rabbitMQ --priority 500
az vm open-port --port 5672 --resource-group rabbitMQ --name rabbitMQ --priority 501
