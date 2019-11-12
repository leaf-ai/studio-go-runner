#!/bin/bash -x
[ -z "$MINIO_ACCESS_KEY" ] && { echo "The MINIO_ACCESS_KEY, and MINIO_SECRET_KEY environment variables must be defined."; exit -1 ; }
[ -z "$MINIO_SECRET_KEY" ] && { echo "The MINIO_ACCESS_KEY, and MINIO_SECRET_KEY environment variables must be defined."; exit -1 ; }
[ -z "$RMQ_ADMIN_PASSWORD" ] && { echo "The RMQ_ADMIN_PASSWORD, and RMQ_USER_PASSWORD environment variables must be defined."; exit -1 ; }
[ -z "$RMQ_USER_PASSWORD" ] && { echo "The RMQ_ADMIN_PASSWORD, and RMQ_USER_PASSWORD environment variables must be defined."; exit -1 ; }

[ -z "$minio_resource_group" ] && minio_resource_group=`wget -O - https://frightanic.com/goodies_content/docker-names.php -q`
aws resource-groups get-group --group-name $minio_resource_group 2>/dev/null 1>&2
ERRCODE=$?
[[ $ERRCODE -ne 0 ]] || \
     { echo "The resource group" $minio_resource_group "was already present. This is unexpected."; exit -1 ; }

[ -z "$rmq_resource_group" ] && rmq_resource_group=`wget -O - https://frightanic.com/goodies_content/docker-names.php -q`
aws resource-groups get-group --group-name $rmq_resource_group 2>/dev/null 1>&2
ERRCODE=$?
[[ $ERRCODE -ne 0 ]] || \
     { echo "The resource group" $rmq_resource_group "was already present. This is unexpected."; exit -1 ; }

mkdir -p installer/aws/minio
mkdir -p installer/aws/rabbitmq
wget -q -O installer/aws/minio/README.md https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/aws/minio/README.md
wget -q -O installer/aws/minio/user-data https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/aws/minio/user-data
wget -q -O installer/aws/rabbitmq/README.md https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/azure/rabbitmq/README.md
wget -q -O installer/aws/rabbitmq/user-data https://raw.githubusercontent.com/leaf-ai/studio-go-runner/feature/233_kustomize/cloud/azure/rabbitmq/user-data
aws_region="$(aws configure get region)"
aws_zones="$(aws ec2 describe-availability-zones --query 'AvailabilityZones[].ZoneName' --output text | awk -v OFS="," '$1=$1')"
ami_id=`aws ec2 describe-images --owners 099720109477 --filters 'Name=name,Values=ubuntu/images/hvm-ssd/ubuntu-bionic-18.04-amd64-server-????????' 'Name=state,Values=available' --query 'reverse(sort_by(Images, &CreationDate))[:1].ImageId' --output text`
IFS=',' read -ra aws_zones_array <<< "$aws_zones"
aws_rand_zone=${aws_zones_array[$RANDOM % ${#aws_zones_array[@]} ]}
unset IFS
echo "n" | ssh-keygen -t rsa -b 4096 -N "" -f ~/.ssh/id_rsa
export PUBLIC_KEY=`cat ~/.ssh/id_rsa.pub`
# More information concerning the discover of AMI IDs can be found at https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/finding-an-ami.html#finding-an-ami-aws-cli
aws ec2 describe-key-pairs --key-names Studio-Go-Runner-$USER 2>/dev/null 1>&2
ERRCODE=$?
[[ $ERRCODE -eq 0 ]] || \
    aws ec2 import-key-pair --key-name Studio-Go-Runner-$USER --public-key-material=$PUBLIC_KEY 2>/dev/null 1>&2

echo -e "\nUsing minio resource group" $minio_resource_group
#
query=`echo '{"Type":"TAG_FILTERS_1_0", "Query":"{\"ResourceTypeFilters\":[\"AWS::AllSupported\"],\"TagFilters\":[{\"Key\":\"Group\", \"Values\":[\"${minio_resource_group}\"]}]}"}' | envsubst`
minio_aws_tags="ResourceType=instance,Tags=[{Key=Name,Value=Studio-Go-Runner-${USER}-minio},{Key=Group,Value=${minio_resource_group}}]"
#
envsubst < installer/aws/minio/user-data > minio-user-data-$minio_resource_group
aws resource-groups create-group --name $minio_resource_group --resource-query "${query}" 1>/dev/null
aws ec2 create-security-group --group-name $minio_resource_group --description "Security Group for the minio instance to allow the minio S3 service port (9000)"
aws ec2 authorize-security-group-ingress --group-name $minio_resource_group --protocol tcp --port 9000 --cidr 0.0.0.0/0
aws ec2 run-instances --image-id $ami_id --key-name Studio-Go-Runner-$USER --security-groups $minio_resource_group \
    --instance-type t3a.large --count 1 \
    --placement AvailabilityZone=$aws_rand_zone \
    --tag-specifications ${minio_aws_tags} \
    --block-device-mappings DeviceName=/dev/sdf,Ebs={VolumeSize=100} \
    --user-data file://minio-user-data-$minio_resource_group
### aws resource-groups delete-group --group-name $minio_resource_group ; aws ec2 delete-security-group --group-name $minio_resource_group
#
#envsubst < installer/azure/rabbitmq/user-data > rabbitmq-user-data-$storage_account
#az group create --name rabbitMQ --location $LOCATION
#az storage account create --resource-group rabbitMQ --name $storage_account  --sku standard_lrs
#az vm create --name rabbitMQ --resource-group rabbitMQ --location eastus --os-disk-size-gb 128 --boot-diagnostics-storage $storage_account --authentication-type ssh --generate-ssh-keys --image Canonical:UbuntuServer:18.04-LTS:latest --public-ip-address-allocation static --size Standard_D4s_v3 --custom-data rabbitmq-user-data-$storage_account -o none
#export RMQ_ADDRESS=$(az network public-ip list --resource-group rabbitMQ --query "[].ipAddress" --output tsv)
#az vm open-port --port 15672 --resource-group rabbitMQ --name rabbitMQ --priority 500
#az vm open-port --port 5672 --resource-group rabbitMQ --name rabbitMQ --priority 501
#echo "RabbitMQ IP Address" $RMQ_ADDRESS "Minio server IP Address" $MINIO_ADDRESS

#aws ec2 run-instances --image-id ami-09c6723c6c24250c9 --count 1 --instance-type t2.small --key-name donn-leaf --subnet-id subnet-f8e5d08e --security-group-ids sg-0d0b196158c59584d --user-data file://launchscript.txt --profile leafdev --region us-west-2
echo -e "\nUsing rabbitMQ resource group" $rmq_resource_group
#
query=`echo '{"Type":"TAG_FILTERS_1_0", "Query":"{\"ResourceTypeFilters\":[\"AWS::AllSupported\"],\"TagFilters\":[{\"Key\":\"Group\", \"Values\":[\"${rmq_resource_group}\"]}]}"}' | envsubst`
rmq_aws_tags="ResourceType=instance,Tags=[{Key=Name,Value=Studio-Go-Runner-${USER}-rabbitMQ},{Key=Group,Value=${rmq_resource_group}}]"
#
envsubst < installer/aws/rabbitmq/user-data > rmq-user-data-$rmq_resource_group
aws resource-groups create-group --name $rmq_resource_group --resource-query "${query}" 1>/dev/null
aws ec2 create-security-group --group-name $rmq_resource_group --description "Security Group for the minio instance to allow the minio S3 service port (9000)"
aws ec2 authorize-security-group-ingress --group-name $rmq_resource_group --protocol tcp --port 22 --cidr 0.0.0.0/0
aws ec2 authorize-security-group-ingress --group-name $rmq_resource_group --protocol tcp --port 5672 --cidr 0.0.0.0/0
aws ec2 authorize-security-group-ingress --group-name $rmq_resource_group --protocol tcp --port 15672 --cidr 0.0.0.0/0
aws ec2 run-instances --image-id $ami_id --key-name Studio-Go-Runner-$USER --security-groups $rmq_resource_group \
    --instance-type t3a.2xlarge --count 1 \
    --placement AvailabilityZone=$aws_rand_zone \
    --tag-specifications ${rmq_aws_tags} \
    --user-data file://rmq-user-data-$rmq_resource_group
