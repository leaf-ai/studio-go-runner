// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains some helper functions
// for tracking Context objects usage and lifecycle.

import (
	"context"
	"fmt"
	"runtime/debug"
)

// GetCancelWrapper will provide cancel function with some additional
// tracing capabilities for debugging.
//
func GetCancelWrapper(cancel context.CancelFunc, msg string) context.CancelFunc {

	return func() {
		logger.Debug(fmt.Sprintf("ContextWrapper: CALLING cancel() for %s at: %s", msg, string(debug.Stack())))
		cancel()
	}
}

