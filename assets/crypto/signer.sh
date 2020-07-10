# Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

export PATH=$PATH:/root/.local/bin
export EXP_DIR=`pwd`
echo `pwd` >&2
ls -alcrt
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"
which pip3
which python3
pip3 install pip==20.0.2 --user
pip3 install --upgrade pip
pip3 install pynacl==1.3.0
pip3 install pycryptodome==3.9.7
pip3 install paramiko==2.7.1
python3 $EXP_DIR/signer.py $EXP_DIR/private $EXP_DIR/payload $EXP_DIR/signature
