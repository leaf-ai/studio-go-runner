# Theano and Numpy and basic linear algebra support for multi-threaded applications

## Introduction

Python traditionally has been a difficult language to use for both concurrent and parallel computation.  In order to address parallelism a number of C and C++ libraries have been created to provide concurrent, and in some cases parallelism, https://blog.golang.org/waza-talk.

Python libraries that wish to exploit parallel computation have been created and adopted by many disciplines in computer science, including machine learning frameworks.

The common approach uses a C API to interface with Python clients.  For TensorFlow a computation graph is sent to the API that expresses what the author of the application the dataflow execution to performi, [TensorFlow Architecture](https://github.com/tensorflow/docs/blob/master/site/en/r1/guide/extend/architecture.md).  On GPU platforms this can result in both sequential or parallel execution depending on the experimenters use of a global computer stream, or the use of multi-streaming.

Key here is that the experimenter has to choose the model to be used for compute.  For TensorFlow this is a well trodden path however for other Python libraries and frameworks this is often hard to implement.

## Motivation

This application describes how to configure and use the Numpy library support for concurrent multi-threading.  This case aligns with CPU applications of Numpy and in the context of this note Theano and Numpy used together.

```
import os, sys, time

import numpy
import theano
import theano.tensor as T

os.environ['MKL_NUM_THREADS'] = sys.argv[1]
os.environ['GOTO_NUM_THREADS'] = sys.argv[1]
os.environ['OMP_NUM_THREADS'] = sys.argv[1]
os.environ['THEANO_FLAGS'] = sys.argv[2]
os.environ['OPENMP'] = 'True'
os.environ['openmp_elemwise_minsize'] = '2000'

M=2000
N=500
K=2000
iters=30
order='C'

X  = numpy.array( numpy.random.randn(M, K), dtype=theano.config.floatX, order=order )
W0 = numpy.array( numpy.random.randn(K, N), dtype=theano.config.floatX, order=order )
Y  = numpy.dot( X, W0 )

Xs = theano.shared( X, name='x' )
Ys = theano.shared( Y, name='y' )
Ws = theano.shared( numpy.array(numpy.random.randn(K, N) / (K + N), dtype=theano.config.floatX, order=order), name='w' )

cost = T.sum( (T.dot(Xs, Ws) - Ys) ** 2)

gradient = theano.grad(cost, Ws)

f = theano.function([], theano.shared(0), updates = [(Ws, Ws - 0.0001 * gradient)])

#grace iteration, to make sure everything is compiled and ready
f()

t0 = time.time()
for i in range(iters):
    f()
print( time.time() - t0 )

print( numpy.mean((W0 - Ws.get_value()) ** 2) )
```

with stock numpy
```
time python3 theano_test.py 4 ""
71.65442895889282
0.11249201362233832
python3 theano_test.py 4 ""  78.93s user 0.86s system 100% cpu 1:19.11 total
```

## Intels MKL Support

https://software.intel.com/en-us/distribution-for-python/choose-download/linux

## Open Source BLAS support

Before starting these instructions please ensure that the existing numpy distribution is removed.

```
pip uninstall numpy
```

Begin the installation of Blas libraries available

```
sudo apt-get install -y libopenblas-base libopenblas-dev
pip install numpy==1.16.4 --no-cache-dir --user --upgrade
time python3 theano_test.py 4 ""
3.777170181274414
0.11204666206632136
python3 theano_test.py 4 ""  25.33s user 8.61s system 456% cpu 7.442 total
```

To further verify that numpy has access to the development packages used for the blass libraries code like the following can be used to dump an inventory of the pakcages it recognizes:

```
python 3
Python 3.6.7 (default, May  2 2020, 13:31:07)
[GCC 7.5.0] on linux
Type "help", "copyright", "credits" or "license" for more information.
>>> import numpy as np
>>> np.__config__.show()
blas_mkl_info:
  NOT AVAILABLE
blis_info:
  NOT AVAILABLE
openblas_info:
    libraries = ['openblas', 'openblas']
    library_dirs = ['/usr/lib/x86_64-linux-gnu']
    language = c
    define_macros = [('HAVE_CBLAS', None)]
blas_opt_info:
    libraries = ['openblas', 'openblas']
    library_dirs = ['/usr/lib/x86_64-linux-gnu']
    language = c
    define_macros = [('HAVE_CBLAS', None)]
lapack_mkl_info:
  NOT AVAILABLE
openblas_lapack_info:
    libraries = ['openblas', 'openblas']
    library_dirs = ['/usr/lib/x86_64-linux-gnu']
    language = c
    define_macros = [('HAVE_CBLAS', None)]
lapack_opt_info:
    libraries = ['openblas', 'openblas']
    library_dirs = ['/usr/lib/x86_64-linux-gnu']
    language = c
    define_macros = [('HAVE_CBLAS', None)]

```

https://scipy.github.io/devdocs/building/linux.html#debian-ubuntu

Wheels and binaries : https://github.com/numpy/numpy/issues/11537

Hand selected blas libraries and building numpy https://numpy.org/devdocs/user/building.html
