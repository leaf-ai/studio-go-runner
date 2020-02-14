// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementations of several functions for parsing and handling
// string representations of numeric values for RAM and disk space etc

import (
	"github.com/dustin/go-humanize"
)

// ParseBytes returns a value for the input string.
//
// This function uses the humanize library from github for go.
//
// Typical inputs can include by way of examples '6gb', '6 GB', '6 GiB'.
// Inputs support SI and IEC sizes.  For more information please review
// https://github.com/dustin/go-humanize/blob/master/bytes.go
//
func ParseBytes(val string) (bytes uint64, err error) {
	return humanize.ParseBytes(val)
}
