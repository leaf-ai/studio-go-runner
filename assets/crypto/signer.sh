# Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

echo `pwd` >&2
pip3 install pip==20.0.2 --user
pip3 install virtualenv --user
virtualenv venv -p python3
source venv/bin/activate
pip install --upgrade pip
pip install pynacl==1.3.0
pip install pycryptodome==3.9.7
pip install paramiko==2.7.1
python3 signer.py ./private ./payload ./signature
deactivate
