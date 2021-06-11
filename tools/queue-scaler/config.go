// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file implements a configuration block for the command.

import (
	"flag"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Rhymond/go-money"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

var (
	accessKeyOpt = flag.String("aws-access-key-id", "", "credentials for accessing SQS queues")
	secretKeyOpt = flag.String("aws-secret-access-key", "", "credentials for accessing SQS queues")
	regionOpt    = flag.String("aws-region", "", "The region in which this command will query for queues")
	queueOpt     = flag.String("aws-queue", "^sqs_.*$", "A regular expression for selecting the queues to be queries")

	maxInstCostOpt = flag.String("max-cost", "10.00", "The maximum permitted cost for all machines, in USD")

	kubeconfigOpt = flag.String("kubeconfig", defaultKubeConfig(), "filepath for the Kubernetes configuration file")
)

type Config struct {
	region    string         // AWS region
	secretKey string         // AWS secretKey
	accessKey string         // AWS accessKey
	queue     *regexp.Regexp // Regular expression for queue queries

	maxCost money.Money // The maximim per hour cost permitted to be allocated

	kubeconfig string // The Kubernetes configuration file
}

func parseMoney(displayAmt string) (amt *money.Money, err kv.Error) {
	parts := strings.Split(displayAmt, ".")
	if len(parts) > 2 {
		return amt, kv.NewError("too many cents values").With("amount", displayAmt).With("stack", stack.Trace().TrimRuntime())
	}
	if len(parts) < 1 {
		return amt, kv.NewError("missing dollar value").With("amount", displayAmt).With("stack", stack.Trace().TrimRuntime())
	}
	dollars, errGo := strconv.Atoi(parts[0])
	if errGo != nil {
		return amt, kv.Wrap(errGo).With("amount", displayAmt).With("stack", stack.Trace().TrimRuntime())
	}
	cents := 0
	if len(parts) == 2 {
		if cents, errGo = strconv.Atoi(parts[1]); errGo != nil {
			return amt, kv.Wrap(errGo).With("amount", displayAmt).With("stack", stack.Trace().TrimRuntime())
		}
	}

	return money.New(int64(dollars*100+cents), "USD"), nil
}

func GetDefaultCfg() (cfg *Config, err kv.Error) {

	cost, errGo := parseMoney(*maxInstCostOpt)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	cfg = &Config{
		accessKey: os.ExpandEnv(*accessKeyOpt),
		secretKey: os.ExpandEnv(*secretKeyOpt),
		region:    os.ExpandEnv(*regionOpt),

		maxCost:    *cost,
		kubeconfig: os.ExpandEnv(*kubeconfigOpt),
	}

	if len(*queueOpt) != 0 {
		reg, errGo := regexp.Compile(os.ExpandEnv(*queueOpt))
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		cfg.queue = reg
	}

	if _, err := os.Stat(cfg.kubeconfig); os.IsNotExist(err) {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return cfg, nil
}
