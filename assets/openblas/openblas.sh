eval "$(pyenv init -)"
export PATH=$(pyenv root)/shims:$PATH
eval "$(pyenv virtualenv-init -)"
pip3 install numpy
python3 openblas.py
