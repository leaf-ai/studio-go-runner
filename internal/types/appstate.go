//go:generate enumer -type K8sState -trimprefix K8s

package types

// This package contains the go code for a enumeration that represents the application
// state for the go runner.  This code will be scanned and used by the enumer code generator
// to produce utility methods for the enumeration

// K8sState represents a desired state for the go runner and the lifecycle it has
// for servicing requests
//
type K8sState int

const (
	K8sUnknown           K8sState = iota
	K8sRunning                    // The runner should restart retrieving work and running if it is not doing so
	K8sDrainAndTerminate          // The runner should complete its current outstanding work and then exit
	K8sDrainAndSuspend            // The runner should complete its current outstanding work and then wait for a K8sResume
)
