eval "$(pyenv init -)"
export PATH=$(pyenv root)/shims:$PATH
eval "$(pyenv virtualenv-init -)"
which pip3
which python3
pip3 install --upgrade pip
pip3 install pynacl==1.3.0
pip3 install pycryptodome==3.9.7
python3 encryptor.py ./public.pem "Hello World!" ./payload
