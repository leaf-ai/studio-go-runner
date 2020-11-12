# A minimal model predictor that can be used to access remote test models
# used for general testing but which are not intended to be of
# much utility for making valuable predictions
#
import tensorflow as tf
from tensorflow import keras

import json
import requests

import argparse

parser = argparse.ArgumentParser(description='Perform model predictions')
parser.add_argument('--port', dest='port', help='port is the TCP port for the prediction endpoint')

args = parser.parse_args()

print('TensorFlow version: {}'.format(tf.__version__))
fashion_mnist = keras.datasets.fashion_mnist
(train_images, train_labels), (test_images, test_labels) = fashion_mnist.load_data()

# scale the values to 0.0 to 1.0
test_images = test_images / 255.0

# reshape for feeding into the model
test_images = test_images.reshape(test_images.shape[0], 28, 28, 1)

# Grab an image from the test dataset and then
# create a json string to ask query to the depoyed model
data = json.dumps({"signature_name": "serving_default",
    "instances": test_images[1:2].tolist()})

# headers for the post request
headers = {"content-type": "application/json"}

# make the post request
json_response = requests.post(f'http://127.0.0.1:{args.port}/v1/models/example-model/versions/2:predict',
                              data=data,
                              headers=headers)

# get the predictions
predictions = json.loads(json_response.text)
print(predictions)
