// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"time"

	"github.com/leaf-ai/studio-go-runner/pkg/log"

	"github.com/cenkalti/backoff/v4"
)

// tfxConfig is
func tfxConfig(ctx context.Context, cfgUpdater *Listeners, retries *backoff.ExponentialBackOff, logger *log.Logger) {
	for {
		select {
		case <-time.After(time.Minute):
			tfxScan(ctx, retries, logger)
		case <-ctx.Done():
			return
		}
	}
}

func tfxScan(ctx context.Context, retries *backoff.ExponentialBackOff, logger *log.Logger) {
}
