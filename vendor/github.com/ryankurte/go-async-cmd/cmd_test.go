/**
 * go-cmd exec/command wrapper
 *
 * https://github.com/ryankurte/go-cmd
 * Copyright 2017 Ryan Kurte
 */

package gocmd

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunnable(t *testing.T) {

	t.Run("Can run commands", func(t *testing.T) {
		c := Command("echo", "Hello")

		err := c.Start()
		if err != nil {
			t.Error(err)
		}

		err = c.Wait()
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("Can interrupt and exit commands", func(t *testing.T) {
		c := Command("echo")

		c.OutputChan = make(chan string)

		err := c.Start()
		if err != nil {
			t.Error(err)
		}

		err = c.Exit()
		if err != nil {
			t.Error(err)
		}
	})

	t.Run("Can stream output from commands", func(t *testing.T) {
		c := Command("echo", "Hello World")

		c.OutputChan = make(chan string)

		err := c.Start()
		if err != nil {
			t.Error(err)
		}

		time.Sleep(100 * time.Millisecond)

		line, ok := <-c.OutputChan
		if !ok {
			t.Errorf("Error fetching from channel")
		}
		if line != "Hello World\n" {
			t.Errorf("Unexpected line out: %s", line)
		}

		c.Exit()
	})

	t.Run("Can write input to commands", func(t *testing.T) {
		os.Remove("test.txt")

		c := Command("tee", "test.txt")
		c.InputChan = make(chan string)

		err := c.Start()
		if err != nil {
			t.Error(err)
		}

		testString := "Test String\n"

		c.InputChan <- testString

		time.Sleep(100 * time.Millisecond)

		c.Exit()

		data, err := ioutil.ReadFile("test.txt")
		line := string(data)

		if !strings.Contains(line, testString) {
			t.Errorf("Unexpected line out: %s", line)
		}
	})

	t.Run("Can write input and read output", func(t *testing.T) {

		c := Command("tee")
		c.InputChan = make(chan string)
		c.OutputChan = make(chan string, 1024)

		err := c.Start()
		if err != nil {
			t.Error(err)
		}

		testString := "Test String\n"

		c.InputChan <- testString

		time.Sleep(100 * time.Millisecond)

		line, ok := <-c.OutputChan
		if !ok {
			t.Errorf("Error fetching from channel")
		}
		if !strings.Contains(line, testString) {
			t.Errorf("Unexpected line out: %s", line)
		}

		c.Exit()

	})

}
