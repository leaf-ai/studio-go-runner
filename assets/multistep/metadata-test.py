import sys

import os
import os.path

import json

firstRun = True

try:
    for line in open('../output/output'):
        if "Has run" in line:
            firstRun = False
except:
    pass

# Output some rubbish
print ('Has run')

# Look into the output dir for a file and bomb if not there

if firstRun:
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
