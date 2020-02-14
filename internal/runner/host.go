// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"net"
	"os"

	"github.com/karlmutch/go-fqdn"
)

// This file contains networking and hosting related functions used by the runner for reporting etc
//

// GetHostName returns a human readable host name that contains as much useful context
// as can be gathered
//
func GetHostName() (name string) {

	name = fqdn.Get()
	if 0 != len(name) && name != "unknown" {
		return name
	}

	name, _ = os.Hostname()

	if 0 != len(name) {
		return name
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		os.Stderr.WriteString("Oops: " + err.Error() + "\n")
		os.Exit(1)
	}

	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return (ipnet.IP.String())
			}
		}
	}
	return "unknown"
}
