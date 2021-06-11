// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"testing"
)

func TestSimpleUnits(t *testing.T) {
	units, err := kubernetesUnits("10gb")
	if err != nil {
		t.Fatal(err.Error())
	}
	if units != "10G" {
		t.Fatal("unable to convert from humanize to kubernetes quantity style ", units)
	}
	units, err = kubernetesUnits("10mb")
	if err != nil {
		t.Fatal(err.Error())
	}
	if units != "10M" {
		t.Fatal("unable to convert from humanize to kubernetes quantity style ", units)
	}
}
