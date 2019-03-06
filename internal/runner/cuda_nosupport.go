// +build !cgo NO_CUDA

package runner

import (
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// This file contains the CUDA functions implemented for the cases where
// a platform cannot support the CUDA hardware, and or APIs

var (
	simDevs = cudaDevices{
		Devices: []device{
			//			device{
			//				Name:    "simulated",
			//				UUID:    "0",
			//				MemTot:  16 * 1024 * 1024 * 1024,
			//				MemFree: 16 * 1024 * 1024 * 1024,
			//				MemUsed: 0,
			//			},
		},
	}
)

func getCUDAInfo() (outDevs cudaDevices, err kv.Error) {

	if len(simDevs.Devices) == 0 {
		return simDevs, kv.NewError("CUDA not supported on this platform").With("stack", stack.Trace().TrimRuntime())
	} else {
		return simDevs, nil
	}
}

func HasCUDA() bool {
	return len(simDevs.Devices) > 0
}
