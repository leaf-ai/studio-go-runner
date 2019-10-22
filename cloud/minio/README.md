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

Change the user-data file to contain your generated or preferred SSH administration key.

az login --use-device-code
az group create --name minio --location eastus
az vm create --name minio --resource-group minio --location eastus --data-disk-sizes-gb 10 --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address-allocation static --size Standard_D4s_v3 --custom-data user-data                            
{
  "fqdns": "",
  "id": "/subscriptions/ssssssss-sssssss-sssssss-sssssssssss/resourceGroups/minio/providers/Microsoft.Compute/virtualMachines/minio",
  "location": "eastus",
  "macAddress": "00-0D-3A-55-8F-69",
  "powerState": "VM running",
  "privateIpAddress": "10.0.0.4",
  "publicIpAddress": "52.226.34.123",
  "resourceGroup": "minio",
  "zones": ""
}

az vm open-port --port 9000 --resource-group minio --name minio
ssh ubuntu@52.226.34.123 -i ~/.ssh/id_ed25519

az group delete --name minio
