 Validating the cloud-init schema

 cloud-init devel schema --config-file user-data


 https://blog.simos.info/how-to-preconfigure-lxd-containers-with-cloud-init/

 lxc profile copy default devprofile
 lxc profile show devprofile > devprofile

lxc profile set devprofile user.user-data "$( cat user-data )"

lxc launch --profile devprofile ubuntu:18.04 junk

lxc file pull junk/var/log/cloud-init.log - > /tmp/karl

lxc delete junk --force
