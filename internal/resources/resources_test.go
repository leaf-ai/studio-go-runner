// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package resources

import (
	"os"
	"testing"
)

// Test the resource fit logic to ensure that the resource fit logic is working
//
func TestResourceFit(t *testing.T) {
	rscs, err := NewResources(os.TempDir())
	if err != nil {
		t.Fatal(err.Error())
	}

	serverRsc := rscs.FetchMachineResources()
	testRsc := serverRsc.Clone()

	if fit, err := testRsc.Fit(serverRsc); !fit || err != nil {
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Fatal("equivalent resource blocks did not fit")
	}
}
