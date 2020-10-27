eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"
sudo mkdir /model
sudo chown $USER:$USER /model
pyenv activate serving-bridge-example
python train.py
2020-10-26 13:40:17.626995: W tensorflow/stream_executor/platform/default/dso_loader.cc:55] Could not load dynamic library 'libnvinfer.so.6'; dlerror: libnvinfer.so.6: cannot open sha
red object file: No such file or directory
2020-10-26 13:40:17.627115: W tensorflow/stream_executor/platform/default/dso_loader.cc:55] Could not load dynamic library 'libnvinfer_plugin.so.6'; dlerror: libnvinfer_plugin.so.6: c
annot open shared object file: No such file or directory
2020-10-26 13:40:17.627144: W tensorflow/compiler/tf2tensorrt/utils/py_utils.cc:30] Cannot dlopen some TensorRT libraries. If you would like to use Nvidia GPU with TensorRT, please ma
ke sure the missing libraries mentioned above are installed properly.
TensorFlow version: 2.1.0

train_images.shape: (60000, 28, 28, 1), of float64
test_images.shape: (10000, 28, 28, 1), of float64
2020-10-26 13:40:20.191333: I tensorflow/stream_executor/platform/default/dso_loader.cc:44] Successfully opened dynamic library libcuda.so.1
2020-10-26 13:40:20.192755: E tensorflow/stream_executor/cuda/cuda_driver.cc:351] failed call to cuInit: CUDA_ERROR_NO_DEVICE: no CUDA-capable device is detected
2020-10-26 13:40:20.192789: I tensorflow/stream_executor/cuda/cuda_diagnostics.cc:156] kernel driver does not appear to be running on this host (awsdev): /proc/driver/nvidia/version d
oes not exist
2020-10-26 13:40:20.193119: I tensorflow/core/platform/cpu_feature_guard.cc:142] Your CPU supports instructions that this TensorFlow binary was not compiled to use: AVX2 AVX512F FMA
2020-10-26 13:40:20.198631: I tensorflow/core/platform/profile_utils/cpu_utils.cc:94] CPU Frequency: 2499995000 Hz
2020-10-26 13:40:20.199017: I tensorflow/compiler/xla/service/service.cc:168] XLA service 0x556ec7667e40 initialized for platform Host (this does not guarantee that XLA will be used).
 Devices:
2020-10-26 13:40:20.199053: I tensorflow/compiler/xla/service/service.cc:176]   StreamExecutor device (0): Host, Default Version
Model: "sequential"
_________________________________________________________________
Layer (type)                 Output Shape              Param #
=================================================================
Conv1 (Conv2D)               (None, 13, 13, 8)         80
_________________________________________________________________
flatten (Flatten)            (None, 1352)              0
_________________________________________________________________
Softmax (Dense)              (None, 10)                13530
=================================================================
Total params: 13,610
Trainable params: 13,610
Non-trainable params: 0
_________________________________________________________________
Train on 60000 samples
Epoch 1/5
60000/60000 [==============================] - 7s 112us/sample - loss: 0.5633 - accuracy: 0.8031
Epoch 2/5
60000/60000 [==============================] - 6s 102us/sample - loss: 0.4440 - accuracy: 0.8454
Epoch 3/5
60000/60000 [==============================] - 6s 105us/sample - loss: 0.3980 - accuracy: 0.8605
Epoch 4/5
60000/60000 [==============================] - 6s 103us/sample - loss: 0.3703 - accuracy: 0.8709
Epoch 5/5
60000/60000 [==============================] - 6s 106us/sample - loss: 0.3528 - accuracy: 0.8754
10000/10000 [==============================] - 1s 72us/sample - loss: 0.3793 - accuracy: 0.8677

Test accuracy: 0.8676999807357788
export_path = /model/1

2020-10-26 13:40:53.667920: W tensorflow/python/util/util.cc:319] Sets are not currently considered sequences, but this may change in the future, so consider avoiding using them.
WARNING:tensorflow:From /home/kmutch/.pyenv/versions/serving-bridge-example/lib/python3.6/site-packages/tensorflow_core/python/ops/resource_variable_ops.py:1786: calling BaseResourceVariable.__init__ (from tensorflow.python.ops.resource_variable_ops) with constraint is deprecated and will be removed in a future version.
Instructions for updating:
If using Keras pass *_constraint arguments to layers.
