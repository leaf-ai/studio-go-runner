// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-stack/stack"
	"github.com/go-test/deep"
	"github.com/jjeffery/kv"
	"github.com/rs/xid"
)

func TestRoundtripTFXCfg(t *testing.T) {
	fn := filepath.Join(*topDir, "assets", "tfx_serving", "cfg.example")
	cfg, err := ReadTFXCfg(fn)
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
	tmpFn := filepath.Join(tmpDir, xid.New().String())
	if err := WriteTFXCfg(tmpFn, cfg); err != nil {
		t.Fatal(err)
	}

	// Reread the temporary file to see if it can be parsed in a round trip
	tmpCfg, err := ReadTFXCfg(tmpFn)
	if err != nil {
		t.Fatal(err)
	}

	// Now compare the parsed versions of the input and then the
	// round tripped saved and re-read configuration
	if diff := deep.Equal(cfg, tmpCfg); diff != nil {
		t.Fatal(diff)
	}
}
