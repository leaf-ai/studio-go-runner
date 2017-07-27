package main

import (
"github.com/mgutz/logxi/v1"
)

var (
	logger log.Logger
)

func init() {
	logger = log.New("runner")
}

func main() {
	logger.Info("Hello World!")
}
