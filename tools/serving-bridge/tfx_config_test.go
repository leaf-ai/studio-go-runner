// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andreidenissov-cog/go-service/pkg/log"
	"github.com/andreidenissov-cog/go-service/pkg/server"
	"github.com/go-stack/stack"
	"github.com/go-test/deep"
	"github.com/jjeffery/kv"
	"github.com/karlmutch/k8s"
	core "github.com/karlmutch/k8s/apis/core/v1"
	meta "github.com/karlmutch/k8s/apis/meta/v1"
	serving_config "github.com/leaf-ai/studio-go-runner/internal/gen/tensorflow_serving/config"
	"github.com/rs/xid"
)

// SetupTfxCfgTest is intended to block until such time as a testing minio server is
// found.  It will also update the server CLI config items to reflect the servers presence.
//
func SetupTfxCfgTest(ctx context.Context, cfgUpdater *Listeners, logger *log.Logger) (err kv.Error) {

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

	testCfg := Config{
		tfxConfigFn: filepath.Join(tmpDir, xid.New().String()),
	}

	tmpTfxCfg := &serving_config.ModelServerConfig{}
	if err = WriteTFXCfg(context.Background(), testCfg, tmpTfxCfg, logger); err != nil {
		return err
	}

	// Send a configuration update
	cfg := ConfigOptionals{
		tfxConfigFn: &testCfg.tfxConfigFn,
	}

	select {
	case cfgUpdater.SendingC <- cfg:
	case <-ctx.Done():
	}
	return nil
}

// TestRoundTripTFXCfgFile tests the ability of the server to load and write as a round trip
// a TFX configuration using a standard file
//
func TestRoundTripTFXCfgFile(t *testing.T) {
	testCfg := Config{
		tfxConfigFn: filepath.Join(*topDir, "assets", "tfx_serving", "cfg.example"),
	}

	cfg, err := ReadTFXCfg(context.Background(), testCfg, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Prepare a temporary output file
	tmpDir, errGo := ioutil.TempDir("", "tfxCfg")
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer os.RemoveAll(tmpDir)

	// Serialize the in memory tfx configuration to a protobuftext file
	testWriteCfg := Config{
		tfxConfigFn: filepath.Join(tmpDir, xid.New().String()),
	}

	if err := WriteTFXCfg(context.Background(), testWriteCfg, cfg, logger); err != nil {
		t.Fatal(err)
	}

	// Reread the temporary file to see if it can be parsed in a round trip
	tmpCfg, err := ReadTFXCfg(context.Background(), testWriteCfg, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Now compare the parsed versions of the input and then the
	// round tripped saved and re-read configuration
	if diff := deep.Equal(cfg, tmpCfg); diff != nil {
		t.Fatal(diff)
	}
}

// TestRoundTripTFXCfgConfigMap tests the ability of the server to load and write as a round trip
// a TFX configuration a Kubernetes ConfigMap
//
func TestRoundTripTFXCfgConfigMap(t *testing.T) {
	if err := server.IsAliveK8s(); err != nil {
		t.Skip(err.Error())
	}

	cmNames := []string{}

	defer func() {
		k8sClient, errGo := k8s.NewInClusterClient()
		if errGo != nil {
			logger.Warn(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).Error())
		}

		configMap := &core.ConfigMap{
			Metadata: &meta.ObjectMeta{
				Namespace: k8s.String(k8sClient.Namespace),
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		for _, cmName := range cmNames {
			configMap.Metadata.Name = k8s.String(cmName)
			// Upsert a k8s config map that we can use for testing purposes
			if errGo = k8sClient.Delete(ctx, configMap); errGo != nil {
				logger.Warn(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).Error())
			}
		}
	}()

	testCfg := Config{
		tfxConfigCM: xid.New().String(),
	}
	cmNames = append(cmNames, testCfg.tfxConfigCM)

	cfg, err := ReadTFXCfg(context.Background(), testCfg, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Prepare a temporary output file
	tmpDir, errGo := ioutil.TempDir("", "tfxCfg")
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer os.RemoveAll(tmpDir)

	// Serialize the in memory tfx configuration to a protobuftext file
	testWriteCfg := Config{
		tfxConfigCM: xid.New().String(),
	}
	cmNames = append(cmNames, testWriteCfg.tfxConfigCM)

	if err := WriteTFXCfg(context.Background(), testWriteCfg, cfg, logger); err != nil {
		t.Fatal(err)
	}

	// Reread the temporary file to see if it can be parsed in a round trip
	tmpCfg, err := ReadTFXCfg(context.Background(), testWriteCfg, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Now compare the parsed versions of the input and then the
	// round tripped saved and re-read configuration
	if diff := deep.Equal(cfg, tmpCfg); diff != nil {
		t.Fatal(diff)
	}
}
