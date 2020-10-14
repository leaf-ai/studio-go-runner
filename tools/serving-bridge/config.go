// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"

	"github.com/jjeffery/kv"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
)

// This file contains the implementation of a configuration block for this
// server

type Config struct {
	endpoint  string
	secretKey string
	accessKey string
	bucket    string
}

// WaitForMinioTest is intended to block until such time as a testing minio server is
// found.  It will also update the server CLI config items to reflect the servers presence.
//
func WaitForMinioTest(ctx context.Context, cfgUpdater *Listeners) (alive bool, err kv.Error) {

	if alive, err := runner.MinioTest.IsAlive(ctx); !alive || err != nil {
		return false, err
	}

	logger.Trace("server minio details", "cmd line", *endpointOpt, "effective", runner.MinioTest.Address)

	if cfgUpdater != nil {
		cfg := ConfigOptionals{
			endpoint:  &runner.MinioTest.Address,
			accessKey: &runner.MinioTest.AccessKeyId,
			secretKey: &runner.MinioTest.SecretAccessKeyId,
		}
		cfgUpdater.SendingC <- cfg
	}
	return true, nil
}
