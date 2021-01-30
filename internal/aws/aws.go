// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package aws_int

// This file contains the implementation of functions related to AWS.
//
// Especially functions related to the credentials file handling

import (
	"bytes"
	"os"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// IsAWS can detect if pods running within a Kubernetes cluster are actually being hosted on an EC2 instance
//
func IsAWS() (aws bool, err kv.Error) {
	fn := "/sys/devices/virtual/dmi/id/product_uuid"
	uuidFile, errGo := os.Open(fn)
	if errGo != nil {
		return false, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", fn)
	}
	defer uuidFile.Close()

	signature := []byte{'E', 'C', '2'}
	buffer := make([]byte, len(signature))

	cnt, errGo := uuidFile.Read(buffer)
	if errGo != nil {
		return false, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", fn)
	}
	if cnt != len(signature) {
		return false, kv.NewError("invalid signature").
			With("file", fn, "buffer", string(buffer), "cnt", cnt).
			With("stack", stack.Trace().TrimRuntime())
	}

	return 0 == bytes.Compare(buffer, signature), nil
}
