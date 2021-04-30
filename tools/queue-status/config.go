// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file implements a configuration block for the command.

import (
	"flag"
	"os"
	"regexp"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

var (
	accessKeyOpt = flag.String("aws-access-key-id", "", "credentials for accessing SQS queues")
	secretKeyOpt = flag.String("aws-secret-access-key", "", "credentials for accessing SQS queues")
	regionOpt    = flag.String("aws-region", "", "The region in which this command will query for queues")
	queueOpt     = flag.String("aws-queue", "^sqs_$", "A regular expression for selecting the queues to be queries")
)

type Config struct {
	region    string         // AWS region
	secretKey string         // AWS secretKey
	accessKey string         // AWS accessKey
	queue     *regexp.Regexp // Regular expression for queue queries
}

func GetDefaultCfg() (cfg *Config, err kv.Error) {
	cfg = &Config{
		accessKey: os.ExpandEnv(*accessKeyOpt),
		secretKey: os.ExpandEnv(*secretKeyOpt),
		region:    os.ExpandEnv(*regionOpt),
	}
	if len(*queueOpt) != 0 {
		reg, errGo := regexp.Compile(*queueOpt)
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		cfg.queue = reg
	}
	return cfg, nil
}
