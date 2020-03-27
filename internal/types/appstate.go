//go:generate enumer -type K8sState -trimprefix K8s

// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package types

// This package contains the go code for a enumeration that represents the application
// state for the go runner.  This code will be scanned and used by the enumer code generator
// to produce utility methods for the enumeration

// K8sState represents a desired state for the go runner and the lifecycle it has
// for servicing requests
//
type K8sState int

const (
	// K8sUnknown indicates that the desired state for the runner is not accessible at this time
	K8sUnknown K8sState = iota
	// K8sRunning inidcates the runner should restart retrieving work and running if it is not doing so
	K8sRunning
	// K8sDrainAndTerminate indicates the runner should complete its current outstanding work and then exit
	K8sDrainAndTerminate
	// K8sDrainAndSuspend indicates the runner should complete its current outstanding work and then wait for a K8sResume
	K8sDrainAndSuspend
)
