// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains some helper functions
// for tracking Context objects usage and lifecycle.

import (
	"context"
	"fmt"
	"github.com/andreidenissov-cog/go-service/pkg/log"
	"github.com/go-stack/stack"
)

// GetCancelWrapper will provide cancel function with some additional
// tracing capabilities for debugging.
//
func GetCancelWrapper(cancel context.CancelFunc, msg string, logger *log.Logger) context.CancelFunc {

	return func() {
		logger.Debug(fmt.Sprintf("ContextWrapper: CALLING cancel() for %s at: %v", msg, stack.Trace().TrimRuntime()))
		cancel()
	}
}
