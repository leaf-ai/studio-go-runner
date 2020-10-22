// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/encoding/prototext"

	serving_config "github.com/leaf-ai/studio-go-runner/internal/gen/tensorflow_serving/config"
	"github.com/rs/xid"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// SetupTfxCfgTest is intended to block until such time as a testing minio server is
// found.  It will also update the server CLI config items to reflect the servers presence.
//
func SetupTfxCfgTest(ctx context.Context, cfgUpdater *Listeners) (err kv.Error) {

	// Prepare a temporary output file
	tmpDir, errGo := ioutil.TempDir("", "tfxTestCfg")
	if errGo != nil {
		logger.Fatal("", "error", errGo, "stack", stack.Trace().TrimRuntime())
	}

	// Only cleanup when the system is shutdown as the TFX config file
	// will be used throughout the lifetime of the server
	go func() {
		<-ctx.Done()
		os.RemoveAll(tmpDir)
	}()

	tmpTfxCfgFn := filepath.Join(tmpDir, xid.New().String())

	tmpTfxCfg := &serving_config.ModelServerConfig{}
	if err = WriteTFXCfg(tmpTfxCfgFn, tmpTfxCfg); err != nil {
		return err
	}

	// Send a configuration update
	cfg := ConfigOptionals{
		tfxConfigFn: &tmpTfxCfgFn,
	}

	select {
	case cfgUpdater.SendingC <- cfg:
	case <-ctx.Done():
	}
	return nil
}

// This file contains the implementation of TFX Model serving configuration
// handling functions

// ReadTFXCfg is used to read the TFX serving configuration file and parse it into a format
// that can be used internally for dealing with model descriptions
//
func ReadTFXCfg(fn string) (cfg *serving_config.ModelServerConfig, err kv.Error) {
	fp, errGo := os.Open(fn)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer fp.Close()

	data, errGo := ioutil.ReadAll(fp)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	cfg = &serving_config.ModelServerConfig{}

	// Unmarshal the text into the struct
	if errGo = prototext.Unmarshal(data, cfg); errGo != nil {
		return nil, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	return cfg, nil
}

// WriteTFXCfg is used to output the models configured for serving by TFX to an
// ASCII format protobuf file.
//
func WriteTFXCfg(fn string, cfg *serving_config.ModelServerConfig) (err kv.Error) {
	opts := prototext.MarshalOptions{
		Multiline: true,
	}
	// Marshall the protobuf data structure into prototext format output
	data, errGo := opts.Marshal(cfg)
	if errGo != nil {
		return kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = ioutil.WriteFile(fn, data, 0600); errGo != nil {
		return kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}
