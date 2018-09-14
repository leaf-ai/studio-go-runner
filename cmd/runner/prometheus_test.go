package main

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

// This file contains the implementation of a prometheus test
// probe that will check the servers metrics resource for metrics
// data that test cases need to validate expected behavior within
// the server logic

func outputMetrics(metricsURL string) (err errors.Error) {

	resp, errGo := http.Get(metricsURL)
	if errGo != nil {
		return errors.Wrap(errGo).With("URL", metricsURL).With("stack", stack.Trace().TrimRuntime())
	}
	defer resp.Body.Close()

	body, errGo := ioutil.ReadAll(resp.Body)
	if errGo != nil {
		return errors.Wrap(errGo).With("URL", metricsURL).With("stack", stack.Trace().TrimRuntime())
	}

	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "runner_cache_") {
			logger.Info(line)
		}
	}
	return nil
}

func getHitsMisses(metricsURL string, hash string) (hits int, misses int, err errors.Error) {
	hits = 0
	misses = 0

	resp, errGo := http.Get(metricsURL)
	if errGo != nil {
		return -1, -1, errors.Wrap(errGo).With("URL", metricsURL).With("stack", stack.Trace().TrimRuntime())
	}
	defer resp.Body.Close()

	body, errGo := ioutil.ReadAll(resp.Body)
	if errGo != nil {
		return -1, -1, errors.Wrap(errGo).With("URL", metricsURL).With("stack", stack.Trace().TrimRuntime())
	}

	hashData := "hash=\"" + hash + "\""
	for _, line := range strings.Split(string(body), "\n") {
		if strings.Contains(line, hashData) && strings.HasPrefix(line, "runner_cache") {
			values := strings.Split(line, " ")
			switch {
			case strings.HasPrefix(line, "runner_cache_hits{"):
				hits, _ = strconv.Atoi(values[len(values)-1])
			case strings.HasPrefix(line, "runner_cache_misses{"):
				misses, _ = strconv.Atoi(values[len(values)-1])
			}
		}
	}
	return hits, misses, nil
}
