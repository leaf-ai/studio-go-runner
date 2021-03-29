// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import "math"

// This file contains small functions related to float handling and comparison logic

// Epsilon was chosen by examining the contents of https://en.wikipedia.org/wiki/Machine_epsilon
// The value was chosen to catch float e upto decimal 32 format which looks to encapsulate the go
// implementation
const float64EqualityThreshold = 1e-6

func almostEqual(a float64, b float64) (close bool) {
	return math.Abs(a-b) <= float64EqualityThreshold
}
