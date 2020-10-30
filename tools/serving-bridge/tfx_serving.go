// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of a TFX configuration generator.  It will listen for model serving
// requests and will regenerate a TFX Serving configuration file to match.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-stack/stack"
	serving_config "github.com/leaf-ai/studio-go-runner/internal/gen/tensorflow_serving/config"
	"github.com/leaf-ai/studio-go-runner/pkg/log"
	"github.com/mitchellh/copystructure"
	"go.opentelemetry.io/otel/api/global"

	"github.com/cenkalti/backoff/v4"
	mapset "github.com/deckarep/golang-set"
)

var (
	tfxStartSync = make(chan struct{})
	tfxEndSync   = make(chan struct{})
)

// TFXScanWait will block the caller until at least one complete update cycle
// is done
func TFXScanWait(ctx context.Context) {

	select {
	case <-ctx.Done():
	case <-tfxStartSync:
	}

	select {
	case <-ctx.Done():
	case <-tfxEndSync:
	}
}

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
			logger.Trace("not ready", "fn", cfg.tfxConfigFn, "error", errGo, "stack", stack.Trace().TrimRuntime())
			return false
		}

		info, errGo := os.Stat(fp)
		if errGo != nil {
			logger.Trace("not ready", "fn", cfg.tfxConfigFn, "error", errGo, "stack", stack.Trace().TrimRuntime())
			return false
		}
		if info.IsDir() {
			logger.Trace("not ready", "fn", cfg.tfxConfigFn, "error", errGo, "stack", stack.Trace().TrimRuntime())
			return false
		}

		logger.Debug("ready", "fn", cfg.tfxConfigFn, "stack", stack.Trace().TrimRuntime())
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
		case <-ticker.C:
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
	// Use 2 channels to denote the start and completion of this function.  The channels being closed will
	// cause any and all listeners to receive a nil and reads to fail.  Listeners should listen to the start
	// channel close and then the end channels closing in order to be sure that the entire cycle of refreshing
	// the state of the server has been completed.
	//
	func() {
		defer func() {
			recover()
			tfxStartSync = make(chan struct{})
		}()
		close(tfxStartSync)
	}()

	defer func() {
		defer func() {
			recover()
			tfxEndSync = make(chan struct{})
		}()
		close(tfxEndSync)
	}()

	_, span := global.Tracer(tracerName).Start(ctx, "tfx-scan-cfg")
	defer span.End()

	// Parse the current TFX configuration
	tfxCfg, err := ReadTFXCfg(ctx, cfg, logger)
	if err != nil {
		logger.Warn("TFX serving configuration could not be read", "error", err, "stack", stack.Trace().TrimRuntime())
		return nil
	}

	logger.Debug(SpewSmall.Sdump(tfxCfg), "stack", stack.Trace().TrimRuntime())

	// Extract out model locations from the configuration we just read
	tfxDirs := mapset.NewSet()

	if tfxCfg.GetModelConfigList() != nil {
		for _, aConfig := range tfxCfg.GetModelConfigList().GetConfig() {
			tfxDirs.Add(aConfig.GetBasePath())
		}
	}

	// Get the set of base directories inside the model index
	mdlDirs := mapset.NewSet()
	{
		mdlBases, err := GetModelIndex().GetBases()
		if err != nil {
			logger.Warn("model retrieve failed", "error", err)
			return nil
		}

		for _, mdlBase := range mdlBases {
			mdlDirs.Add(mdlBase)
		}
	}

	// Visit the known models at this point and look into the TFX serving cfg structure
	// to align it with the models

	// Any tfx dirs that are not in the model dirs treat as deletes
	deletions := tfxDirs.Difference(mdlDirs)

	// Any model dirs that are not in tfx dirs treat as additions
	additions := mdlDirs.Difference(tfxDirs)

	if deletions.Cardinality() == 0 && additions.Cardinality() == 0 {
		logger.Debug("debug", "stack", stack.Trace().TrimRuntime())
		return nil
	}

	if logger.IsDebug() {
		logger.Debug(SpewSmall.Sdump(deletions), "stack", stack.Trace().TrimRuntime())
		logger.Debug(SpewSmall.Sdump(additions), "stack", stack.Trace().TrimRuntime())
	}

	for _, deletion := range deletions.ToSlice() {
		cfgs := tfxCfg.GetModelConfigList().GetConfig()
		for i, cfg := range cfgs {
			if deletion == cfg.GetBasePath() {
				cfgs = append(cfgs[:i], cfgs[i+1:]...)
				tfxCfg.GetModelConfigList().Config = cfgs
				break
			}
		}
	}

	cfgList, _ := tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList)
	if cfgList == nil {
		tfxCfg.Config = &serving_config.ModelServerConfig_ModelConfigList{}
	}
	cfgs := make([]*serving_config.ModelConfig, 0, len(additions.ToSlice()))

	for _, addition := range additions.ToSlice() {
		addName := addition.(string)
		mdl := &serving_config.ModelConfig{
			BasePath:      addName,
			ModelPlatform: "tensorflow",
		}
		cfgs = append(cfgs, mdl)
		logger.Debug(SpewSmall.Sdump(tfxCfg.GetModelConfigList()), "stack", stack.Trace().TrimRuntime())
	}
	if len(cfgs) != 0 {
		if tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList == nil {
			tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList = &serving_config.ModelConfigList{
				Config: []*serving_config.ModelConfig{},
			}
		}
		list := tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList
		list.Config = append(list.Config, cfgs...)

		logger.Debug(Spew.Sdump(tfxCfg), "stack", stack.Trace().TrimRuntime())
		logger.Debug(fmt.Sprintf("%#v", tfxCfg), "stack", stack.Trace().TrimRuntime())
		logger.Debug(fmt.Sprintf("%#v", tfxCfg.Config), "stack", stack.Trace().TrimRuntime())
		logger.Debug(fmt.Sprintf("%#v", tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList), "stack", stack.Trace().TrimRuntime())
		logger.Debug(fmt.Sprintf("%#v", tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList.Config), "stack", stack.Trace().TrimRuntime())
		for _, mdlCfg := range tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList.Config {
			logger.Debug(mdlCfg.BasePath, "stack", stack.Trace().TrimRuntime())
		}

	}
	// Generate an updated TFX configuration
	if err := WriteTFXCfg(ctx, cfg, tfxCfg, logger); err != nil {
		logger.Warn("TFX serving configuration could not be modified", "error", err)
		return nil
	}

	return tfxCfg
}
