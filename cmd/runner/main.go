package main

import (
	"fmt"

	"github.com/karlmutch/studio-go-runner"

	"github.com/mgutz/logxi/v1"
)

var (
	logger log.Logger
)

func init() {
	logger = log.New("runner")
}

func main() {

	pubSub, err := runner.NewPubSub("tfstudio")
	if Err != nil {
		logger.Fatal(fmt.Sprintf("could not start the pubsub listener due to %v", err))
	}
}
