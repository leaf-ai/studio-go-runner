import sys
import time

import os
import os.path

import json

firstRun = True

def touch(fname, times=None):
    with open(fname, 'a'):
        os.utime(fname, times)

try:
    if os.path.isfile('/tmp/firstRun'):
        firstRun = False
except:
    pass

# Output some rubbish
touch('/tmp/firstRun')

# Look into the output dir for a file and wait until the job expires, and if that
# fails then bailout with an error

if firstRun:
    time.sleep(120)
    sys.exit(-1)

data = {
    "experiment": {
        "name": "Zaphod Beeblebrox",
    }
}

# Output useful metadata
print (json.dumps(data))

# On the second and subsequent runs if any stop cleanly
sys.exit(0)
