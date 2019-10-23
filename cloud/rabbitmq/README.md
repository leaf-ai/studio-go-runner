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

# Azure deployment

Change the user-data file to contain your generated or preferred SSH administration key, in this case of this example we pasted in the contents of the $HOME/.ssh/id_ed25519.pub file.

You will also need to modify the user-data to specify the admin_password, and the user_password within the user-data configuration file.  These should be protected secrets that you choose and will need to pass to anyone wishing to write and read messages on the queue server.

az login --use-device-code
storage_account=`echo -e "bootlogs$(uuidgen | md5sum - | cut -c1-8)"`
az group create --name rabbitMQ --location eastus
az storage account create --resource-group rabbitMQ --name $storage_account  --sku standard_lrs
az vm create --name rabbitMQ --resource-group rabbitMQ --location eastus --os-disk-size-gb 128 --boot-diagnostics-storage $storage_account --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address-allocation static --size Standard_D4s_v3 --custom-data user-data
{
  "fqdns": "",
  "id": "/subscriptions/ssssssss-sssssss-sssssss-sssssssssss/resourceGroups/rabbitMQ/providers/Microsoft.Compute/virtualMachines/rabbitMQ",
  "location": "eastus",
  "macAddress": "00-0D-3A-55-8F-69",
  "powerState": "VM running",
  "privateIpAddress": "10.0.0.4",
  "publicIpAddress": "52.226.34.123",
  "resourceGroup": "rabbitMQ",
  "zones": ""
}

az vm open-port --port 15672 --resource-group rabbitMQ --name rabbitMQ --priority 500
az vm open-port --port 5672 --resource-group rabbitMQ --name rabbitMQ --priority 501

## Shell access
ssh ubuntu@52.226.34.123 -i ~/.ssh/id_ed25519

## Diagnostics

az vm boot-diagnostics get-boot-log --ids $(az vm list -g rabbitMQ --query "[].id" -o tsv)
