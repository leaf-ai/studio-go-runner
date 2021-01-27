// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of a TFX configuration generator.  It will listen for model serving
// requests and will regenerate a TFX Serving configuration file to match.

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/go-service/pkg/log"
	serving_config "github.com/leaf-ai/studio-go-runner/internal/gen/tensorflow_serving/config"
	"github.com/mitchellh/copystructure"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

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

	_, span := otel.Tracer(tracerName).Start(ctx, "tfx-lifecycle")
	defer span.End()

	logger.Debug("tfxConfig waiting for config")

	// Define a validation function for this component is be able to begin running
	// that tests for completeness of the first received configuration updates
	readyF := func(ctx context.Context, cfg Config, logger *log.Logger) (isValid bool) {
		_, span := otel.Tracer(tracerName).Start(ctx, "tfx-start-validate")
		defer span.End()

		// Special case for empty configuration files that are present but which wont parse as valid config files until this
		// server is running
		if len(cfg.tfxConfigFn) != 0 {
			err := func() (err kv.Error) {
				fp, errGo := filepath.Abs(cfg.tfxConfigFn)
				if errGo != nil {
					return kv.Wrap(errGo).With("fn", cfg.tfxConfigFn, "stack", stack.Trace().TrimRuntime())
				}

				info, errGo := os.Stat(fp)
				if errGo != nil {
					return kv.Wrap(errGo).With("fn", cfg.tfxConfigFn, "stack", stack.Trace().TrimRuntime())
				}
				if info.IsDir() {
					return kv.Wrap(errGo).With("fn", cfg.tfxConfigFn, "stack", stack.Trace().TrimRuntime())
				}

				logger.Debug("ready", "fn", cfg.tfxConfigFn, "stack", stack.Trace().TrimRuntime())
				return nil
			}()
			if err == nil {
				return true
			}
		}

		// See if the standard methods are able to load the TFX Serving configuration file
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		_, err := ReadTFXCfg(ctx, cfg, logger)
		if err == nil {
			logger.Debug("ready", "config_map", cfg.tfxConfigCM, "stack", stack.Trace().TrimRuntime())
			return true
		}
		span.SetStatus(codes.Error, err.Error())
		logger.Debug("not ready", "config_map", cfg.tfxConfigCM, "error", err.Error(), "stack", stack.Trace().TrimRuntime())
		return false
	}

	cfg, updatedCfgC := cfgWatcherStart(ctx, cfgUpdater, readyF, logger)

	logger.Debug("tfxConfig scanning starting")
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

	_, span := otel.Tracer(tracerName).Start(ctx, "tfx-scan")
	defer span.End()

	// We track the file, and the alternative config map names so that if they change we clear
	// our cached configuration (lastTfxCfg)
	lastKnownCfgFn := cfg.tfxConfigFn
	lastKnownCfgCM := cfg.tfxConfigCM
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
			if cfg.tfxConfigFn != lastKnownCfgFn || cfg.tfxConfigCM != lastKnownCfgCM {
				cfg.tfxConfigFn = lastKnownCfgFn
				cfg.tfxConfigCM = lastKnownCfgCM
				lastTfxCfg = &serving_config.ModelServerConfig{}
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

	_, span := otel.Tracer(tracerName).Start(ctx, "tfx-scan-cfg")
	defer span.End()

	// Parse the current TFX configuration, can be read from S3, files and config maps
	tfxCfg, err := ReadTFXCfg(ctx, cfg, logger)
	if err != nil {
		logger.Warn("TFX serving configuration could not be read", "error", err, "stack", stack.Trace().TrimRuntime())
		return nil
	}

	// Extract out model locations from the configuration we just read
	tfxDirs := mapset.NewSet()

	if tfxCfg.GetModelConfigList() != nil {
		for _, aConfig := range tfxCfg.GetModelConfigList().GetConfig() {
			tfxDirs.Add(aConfig.GetBasePath())
		}
	}

	// Get the set of base directories inside the model index
	names := map[string]string{} // map of model locations to a generated name for them
	mdlDirs := mapset.NewSet()
	{
		mdlBases, err := GetModelIndex().GetBases()
		if err != nil {
			logger.Warn("model retrieve failed", "error", err)
			return nil
		}

		for _, mdlBase := range mdlBases {
			// Adjust the parth from a directory style for a key to a full S3 URI
			loc := "s3://" + cfg.bucket + "/" + mdlBase + "/"
			mdlDirs.Add(loc)
			names[loc] = mdlBase
		}
	}

	// Visit the known models at this point and look into the TFX serving cfg structure
	// to align it with the models

	// Any tfx dirs that are not in the model dirs treat as deletes
	deletions := tfxDirs.Difference(mdlDirs)

	// Any model dirs that are not in tfx dirs treat as additions
	additions := mdlDirs.Difference(tfxDirs)

	if deletions.Cardinality() == 0 && additions.Cardinality() == 0 {
		return nil
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
		if len(addName) == 0 {
			continue
		}
		mdl := &serving_config.ModelConfig{
			BasePath:      addName,
			ModelPlatform: "tensorflow",
		}
		if name, isPresent := names[addName]; isPresent {
			mdl.Name = name
		}
		cfgs = append(cfgs, mdl)
	}
	if len(cfgs) != 0 {
		if tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList == nil {
			tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList = &serving_config.ModelConfigList{
				Config: []*serving_config.ModelConfig{},
			}
		}
		list := tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList
		list.Config = append(list.Config, cfgs...)

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
