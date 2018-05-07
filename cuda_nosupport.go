// +build !cgo NO_CUDA

package runner

// This file contains the CUDA functions implemented for the cases where
// a platform cannot support the CUDA hardware, and or APIs

import (
	"fmt"
)

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

func getCUDAInfo() (outDevs cudaDevices, err error) {

	if len(simDevs.Devices) == 0 {
		return simDevs, fmt.Errorf("CUDA not supported on this platform")
	} else {
		return simDevs, nil
	}
}

func HasCUDA() bool {
	return len(simDevs.Devices) > 0
}
