package main

// This file containes the implementation of a set of functions that will on a
// regular basis output information about the runner that could be useful to observers

import (
	"context"
	"time"

	"github.com/SentientTechnologies/studio-go-runner"
)

func showResources(ctx context.Context) {

	res := &runner.Resources{}

	refresh := time.NewTicker(5 * time.Second)
	defer refresh.Stop()

	showTime := time.NewTicker(time.Minute)
	defer showTime.Stop()

	lastMsg := ""
	nextOutput := time.Now()

	for {
		select {
		case <-refresh.C:
			if msg := res.Dump(); msg != lastMsg {
				logger.Info(msg)
				lastMsg = msg
				nextOutput = time.Now().Add(time.Duration(10 * time.Second))
			}
		case <-showTime.C:
			if !time.Now().Before(nextOutput) {
				lastMsg = res.Dump()
				logger.Info(lastMsg)
				nextOutput = time.Now().Add(time.Duration(30 * time.Second))
			}

		case <-ctx.Done():
			return
		}
	}
}
