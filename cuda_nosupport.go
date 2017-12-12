// +build !cgo NO_CUDA

package runner

// This file contains the CUDA functions implemented for the cases where
// a platform cannot support the CUDA hardware, and or APIs

import (
	"fmt"
)

func getCUDAInfo() (outDevs devices, err error) {
	return devices{}, fmt.Errorf("CUDA not supported on this platform")
}

func HasCUDA() bool {
	return false
}
