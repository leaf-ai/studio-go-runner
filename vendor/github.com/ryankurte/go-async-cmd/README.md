# go-async-cmd

[![Documentation](https://img.shields.io/badge/docs-godoc-blue.svg)](https://godoc.org/github.com/ryankurte/go-async-cmd)
[![GitHub tag](https://img.shields.io/github/tag/ryankurte/go-async-cmd.svg)](https://github.com/ryankurte/go-async-cmd)
[![Build Status](https://travis-ci.org/ryankurte/go-async-cmd.svg?branch=master)](https://travis-ci.org/ryankurte/go-async-cmd)

A wrapper around [exec/cmd](https://golang.org/pkg/os/exec/#Command) that provides asynchronous channels to read and write to a running commmand.


## Usage

```go

import(
    "gopkg.in/ryankurte/go-async-cmd.v1"
)

// Create the command, this matches the syntax of exec/Cmd
c := gocmd.Command("tee")

// Create input and output channels if required
c.InputChan = make(chan string, 1024)
c.OutputChan = make(chan string, 1024)

testString := "Test String\n"

c.InputChan <- testString

time.Sleep(500 * time.Millisecond)

line, ok := <-c.OutputChan
if !ok {
    t.Errorf("Error fetching from channel")
}
if !strings.Contains(line, testString) {
    t.Errorf("Unexpected line out: %s", line)
}

...

// Exit the command. This calls wait, then sends an interrupt and kill at predefined intervals
// to allow exiting from long-running processes
// You can also user wait or any of the other standard methods
sm.Exit()

```