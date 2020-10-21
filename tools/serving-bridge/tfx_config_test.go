// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"path/filepath"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestReadTFXCfg(t *testing.T) {
	fn := filepath.Join(*topDir, "assets", "tfx_serving", "cfg.example")
	cfg, err := ReadTFXCfg(fn)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info(spew.Sdump(cfg))
}
