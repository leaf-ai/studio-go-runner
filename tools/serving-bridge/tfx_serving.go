// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of a TFX configuration generator.  It will listen for model serving
// requests and will regenerate a TFX Serving configuration file to match.

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/leaf-ai/studio-go-runner/pkg/log"

	"github.com/cenkalti/backoff/v4"
)

// tfxConfig is used to initialize the TFX serving configuration management component.
func tfxConfig(ctx context.Context, cfgUpdater *Listeners, retries *backoff.ExponentialBackOff, logger *log.Logger) {

	logger.Debug("tfxConfig waiting for config")

	// Define a validation function for this component is be able to begin running
	// that tests for completeness of the first received configuration updates
	readyF := func(cfg Config) (isValid bool) {
		// Make sure that the fully qualified file name is present
		// and is not a directory
		fp, errGo := filepath.Abs(cfg.tfxConfigFn)
		if errGo != nil {
			return false
		}

		info, errGo := os.Stat(fp)
		if errGo != nil {
			return false
		}
		if info.IsDir() {
			return false
		}

		return true
	}

	cfg, updatedCfgC := cfgWatcherStart(ctx, cfgUpdater, readyF)

	for {
		select {
		case <-time.After(time.Minute):
			tfxScan(ctx, cfg, updatedCfgC, retries, logger)
		case <-ctx.Done():
			return
		}
	}
}

func tfxScan(ctx context.Context, cfg Config, updatedCfgC chan Config, retries *backoff.ExponentialBackOff, logger *log.Logger) {
	logger.Debug("tfxScan initialized using", cfg.tfxConfigFn)

	// Parse the current TFX configuration
	// Diff the TFX configuration with the in memory model catalog
	// Generate an updated TFX configuration
}
