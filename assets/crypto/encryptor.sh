pip3 install pip==20.0.2 --user
pip3 install virtualenv --user
virtualenv venv -p python3
source venv/bin/activate
pip install --upgrade pip
pip install pynacl==1.3.0
pip install pycryptodome==3.9.7
python3 encryptor.py ./public.pem "Hello World!"
