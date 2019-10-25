# Azure deployment

Change the user-data file to contain your generated or preferred SSH administration key, in this case of this example we pasted in the contents of the $HOME/.ssh/id_ed25519.pub file into the user-data file as an item inside the ssh-authorized-keys yaml block.  You can run the following command to generate a key if you are not sure that you have one really.

```shell
echo "n" | ssh-keygen -t ed25519 -N "" -f ~/.ssh/id_ed25519
export PUBLIC_KEY=`cat ~/.ssh/id_ed25519.pub`
```

You will also need to define environment variables to specify the MINIO_ACCESS_KEY, MINIO_SECRET_KEY as environment variables.  These should be protected secrets that you choose and will need to pass to anyone wishing to upload and download data on this server.

```shell
storage_account=`echo -e "bootlogs$(uuidgen | md5sum - | cut -c1-8)"`
export MINIO_ACCESS_KEY=[An access key you choose and is secret to you, and users of StudioML, LEAF]
export MINIO_SECRET_KEY=[A secret key you choose and is secret to you, and users of StudioML, LEAF.  Must be at least 8 characters in length.]
envsubst < user-data > user-data-$storage_account
az login --use-device-code
az group create --name minio --location eastus -o none
az storage account create --resource-group minio --name $storage_account  --sku standard_lrs -o none
az vm create --name minio --resource-group minio --location eastus --data-disk-sizes-gb 10 --boot-diagnostics-storage $storage_account --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address-allocation static --size Standard_D4s_v3 --custom-data user-data-$storage_account -o none
export MINIO_ADDRESS=$(az network public-ip list --resource-group minio --query "[].ipAddress" --output tsv)
az vm open-port --port 9000 --resource-group minio --name minio -o none
```

## Shell access
ssh ubuntu@$MINIO_ADDRESS -i ~/.ssh/id_ed25519

## Diagnostics

az vm boot-diagnostics get-boot-log --ids $(az vm list -g minio --query "[].id" -o tsv)

# Testing

Validating the cloud-init schema

cloud-init devel schema --config-file user-data


https://blog.simos.info/how-to-preconfigure-lxd-containers-with-cloud-init/

lxc profile copy default devprofile
lxc profile show devprofile

lxc profile set devprofile user.user-data "$( cat user-data )"

lxc launch --profile devprofile ubuntu:18.04 junk

lxc file pull junk/var/log/cloud-init.log -

lxc delete junk --force
