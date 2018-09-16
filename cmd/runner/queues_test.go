package main

import (
	rh "github.com/michaelklishin/rabbit-hole"
	"github.com/rs/xid"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

// This file contains the implementation of a test subsystem
// for deploying rabbitMQ in test scenarios where it
// has been installed for the purposes of running end-to-end
// tests related to queue handling and state management

func InitTestQueues() (err errors.Error) {

	if len(*amqpURL) == 0 {
		return errors.New("amqpURL was not specified on the command line, or as an env var, cannot start rabbitMQ").With("stack", stack.Trace().TrimRuntime())
	}

	// Start by making sure that when things were started we saw a rabbitMQ configured
	// on the localhost.  If so then check that the rabbitMQ started automatically as a result of
	// the Dockerfile_full setup
	//
	rmqc, errGo := rh.NewClient("http://127.0.0.1:15672", "guest", "guest")
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// declares a queue
	if _, errGo = rmqc.DeclareQueue("/", xid.New().String(), rh.QueueSettings{Durable: false}); errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}
