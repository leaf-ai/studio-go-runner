import sys
import time

import os
import os.path

import json

def touch(fname, times=None):
    with open(fname, 'a'):
        os.utime(fname, times)

# Look into the output dir for a file and wait until the job expires, and if that
# fails then bailout with an error
try:
    if not os.path.isfile('/tmp/firstRun'):
        touch('/tmp/firstRun')
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
