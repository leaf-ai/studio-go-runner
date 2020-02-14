# Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved.
# Issued under the Apache 2.0 License.

import sys
import time

import os
import os.path

import json

def touch(fname, times=None):
    with open(fname, 'a'):
        os.utime(fname, times)
# During the first pass we will inject a number of directives for document editing

data = { "experiment": {"name": "dummy pass"}}
print(json.dumps(data))

# Look into the output dir for a file and wait until the job expires, and if that
# fails then bailout with an error
try:
    if not os.path.isfile('/tmp/firstRun'):
        touch('/tmp/firstRun')
        edit = [{"op": "replace", "path": "/experiment/name", "value": "First pass"}]
        print(json.dumps(edit))
        edit = [{"op": "remove", "path": "/experiment"}]
        print(json.dumps(edit))
        sys.exit(-1)
except:
    touch('/tmp/firstRun')
    sys.exit(-1)

data = {
    "experiment": {
        "name": "Zaphod Beeblebrox",
    }
}

# Output useful metadata
print (json.dumps(data))
sys.stdout.flush()
