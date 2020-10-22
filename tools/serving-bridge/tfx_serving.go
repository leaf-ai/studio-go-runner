// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of a TFX configuration generator.  It will listen for model serving
// requests and will regenerate a TFX Serving configuration file to match.

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/go-test/deep"
	serving_config "github.com/leaf-ai/studio-go-runner/internal/gen/tensorflow_serving/config"
	"github.com/leaf-ai/studio-go-runner/pkg/log"
	"github.com/mitchellh/copystructure"
	"go.opentelemetry.io/otel/api/global"

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
			logger.Debug("not ready", "fn", cfg.tfxConfigFn, "error", errGo, "stack", stack.Trace().TrimRuntime())
			return false
		}

		info, errGo := os.Stat(fp)
		if errGo != nil {
			logger.Debug("not ready", "fn", cfg.tfxConfigFn, "error", errGo, "stack", stack.Trace().TrimRuntime())
			return false
		}
		if info.IsDir() {
			logger.Debug("not ready", "fn", cfg.tfxConfigFn, "error", errGo, "stack", stack.Trace().TrimRuntime())
			return false
		}

		logger.Info("ready", "fn", cfg.tfxConfigFn, "stack", stack.Trace().TrimRuntime())
		return true
	}

	cfg, updatedCfgC := cfgWatcherStart(ctx, cfgUpdater, readyF)

	logger.Debug("tfxConfig config ready starting")
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

	_, span := global.Tracer(tracerName).Start(ctx, "tfx-scan")
	defer span.End()

	lastKnownCfgFn := cfg.tfxConfigFn
	lastTfxCfg := &serving_config.ModelServerConfig{}

	ticker := backoff.NewTickerWithTimer(retries, nil)
	defer ticker.Stop()

	for {
		select {
		case newCfg := <-updatedCfgC:
			cpy, errGo := copystructure.Copy(newCfg)
			if errGo != nil {
				logger.Warn("updated configuration could not be used", "error", errGo.Error(), "stack", stack.Trace().TrimRuntime())
				continue
			}
			cfg = cpy.(Config)
			// Check to see if the config filename has changed, and if so reset our history
			// which in turn forces a reread of the new configuration change
			if cfg.tfxConfigFn != lastKnownCfgFn {
				cfg.tfxConfigFn = lastKnownCfgFn
				lastTfxCfg = &serving_config.ModelServerConfig{}
				logger.Warn("debug", "stack", stack.Trace().TrimRuntime())
			}
			logger.Debug(spew.Sdump(cfg))
		case <-ticker.C:
			logger.Warn("debug", "stack", stack.Trace().TrimRuntime())
			logger.Debug(spew.Sdump(cfg))

			if err := tfxScanConfig(ctx, lastTfxCfg, cfg, retries, logger); err != nil {
				continue
			}
			return
		case <-ctx.Done():
			return
		}
	}
}

func tfxScanConfig(ctx context.Context, lastTfxCfg *serving_config.ModelServerConfig, cfg Config, retries *backoff.ExponentialBackOff, logger *log.Logger) (updated *serving_config.ModelServerConfig) {
	_, span := global.Tracer(tracerName).Start(ctx, "tfx-scan-cfg")
	defer span.End()

	// Parse the current TFX configuration
	tfxCfg, err := ReadTFXCfg(cfg.tfxConfigFn)
	if err != nil {
		logger.Warn("TFX serving configuration could not be read", "error", err, "stack", stack.Trace().TrimRuntime())
		return nil
	}
	// Diff the TFX configuration with the in memory model catalog and
	// see if anything has changed since the last pass.  If not processing
	// is not needed to just return
	if diff := deep.Equal(lastTfxCfg, tfxCfg); diff == nil {
		return nil
	}

	// Visit the known models at this point and look into the TFX serving cfg structure
	// to align it with the models

	// Generate an updated TFX configuration
	return tfxCfg
}
