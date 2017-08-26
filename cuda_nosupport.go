// +build !cgo NO_CUDA

package runner

// This file contains the CUDA functions implemented for the cases where
// a platform cannot support the CUDA hardware, and or APIs

func getCUDAInfo() (outDevs devices, err error) {
	return fmt.Errof("CUDA not supported on this platform")
}
