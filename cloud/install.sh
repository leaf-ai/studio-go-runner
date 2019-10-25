#!/bin/bash
#
# Change the following three parameters to suit your specific situation.  Choosing the correct LOCATION
# is critical for preventing excessive data transfers costs and should be the same as where you intend
# on deploying your GPU AI infrastructure
#
export LOCATION=eastus
export MINIO_ACCESS_KEY=[An access key you choose and is secret to you, and users of StudioML, LEAF]
export MINIO_SECRET_KEY=[A secret key you choose and is secret to you, and users of StudioML, LEAF.  Must be at least 8 characters in length.]
#
#
# Set passwords that are private to your installation in the next two lines.  Choose passwords that 
# are more than 8 characters.
#
export RMQ_ADMIN_PASSWORD=[A secret key you choose and is secret to the administrator]
export RMQ_USER_PASSWORD=[A secret key you choose and is secret to users of StudioML, or LEAF]
#
/bin/bash azure/install.sh
