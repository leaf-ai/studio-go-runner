package main

import (
	"fmt"
	"time"

	runner "github.com/SentientTechnologies/studio-go-runner"
)

// This file contains the implementation of a RabbitMQ service for
// retriving and handling StudioML workloads within a self hosted
// queue context

func serviceRMQ(connTimeout time.Duration, quitC chan struct{}) {

	live := &Projects{projects: map[string]chan bool{}}

	rmq, err := runner.NewRabbitMQ(*amqpURL, "")
	if err != nil {
		logger.Error(err.Error())
	}

	// first time through make sure the credentials are checked immediately
	qCheck := time.Duration(time.Second)

	for {
		select {
		case <-quitC:
			live.Lock()
			defer live.Unlock()

			// When shutting down stop all projects
			for _, quiter := range live.projects {
				close(quiter)
			}
			return
		case <-time.After(qCheck):
			qCheck = time.Duration(15 * time.Second)

			found, err := rmq.GetKnown(time.Duration(time.Minute))
			if err != nil {
				logger.Warn(fmt.Sprintf("unable to refresh RMQ queue manifest due to %v", err))
			}

			live.Lifecycle(found)
		}
	}
}
