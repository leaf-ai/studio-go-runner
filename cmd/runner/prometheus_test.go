package main

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	"github.com/prometheus/common/expfmt"
)

// This file contains the implementation of a prometheus test
// probe that will check the servers metrics resource for metrics
// data that test cases need to validate expected behavior within
// the server logic

type prometheusClient struct {
	url string
}

func NewPrometheusClient(url string) (cli *prometheusClient) {
	return &prometheusClient{
		url: url,
	}
}

func (p *prometheusClient) Fetch(prefix string) (err errors.Error) {

	resp, errGo := http.Get(p.url)
	if errGo != nil {
		return errors.Wrap(errGo).With("URL", p.url).With("stack", stack.Trace().TrimRuntime())
	}
	defer resp.Body.Close()

	parser := expfmt.TextParser{}
	metricFamilies, errGo := parser.TextToMetricFamilies(resp.Body)
	if errGo != nil {
		return errors.Wrap(errGo).With("URL", p.url).With("stack", stack.Trace().TrimRuntime())
	}
	for k, v := range metricFamilies {
		if len(prefix) == 0 || strings.HasPrefix(k, prefix) {
			logger.Info(k, spew.Sdump(v))
		}
	}
	return nil
}

func (p *prometheusClient) getMetric(prefix string) (items []string, err errors.Error) {

	resp, errGo := http.Get(p.url)
	if errGo != nil {
		return items, errors.Wrap(errGo).With("URL", p.url).With("stack", stack.Trace().TrimRuntime())
	}
	defer resp.Body.Close()

	body, errGo := ioutil.ReadAll(resp.Body)
	if errGo != nil {
		return items, errors.Wrap(errGo).With("URL", p.url).With("stack", stack.Trace().TrimRuntime())
	}

	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if len(prefix) == 0 || strings.HasPrefix(line, prefix) {
			items = append(items, line)
		}
	}
	return items, nil
}

func (p *prometheusClient) GetHitsMisses(hash string) (hits int, misses int, err errors.Error) {
	lines, err := p.getMetric("runner_cache")
	if err != nil {
		return hits, misses, err
	}
	hashData := "hash=\"" + hash + "\""
	for _, line := range lines {
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
