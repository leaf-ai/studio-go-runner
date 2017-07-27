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

	ps, err := runner.NewPubSub("tfstudio")
	if err != nil {
		logger.Fatal(fmt.Sprintf("could not start the pubsub listener due to %v", err))
	}
	ps.Client.Close()
}
