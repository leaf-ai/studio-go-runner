// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"io/ioutil"
	"os"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file contains the implementation of TFX Model serving configuration
// handling functions

func ReadTFXCfg(fn string) (tfxCfg *anypb.Any, err kv.Error) {
	fp, errGo := os.Open(fn)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer fp.Close()

	data, errGo := ioutil.ReadAll(fp)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	tfxCfg = &anypb.Any{}

	// Unmarshal the text into the struct
	if errGo = prototext.Unmarshal(data, tfxCfg); errGo != nil {
		return nil, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	return tfxCfg, nil
}
