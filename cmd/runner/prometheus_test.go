package main

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	dto "github.com/prometheus/client_model/go"
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

func (p *prometheusClient) Fetch(prefix string) (metrics map[string]*dto.MetricFamily, err kv.Error) {

	resp, errGo := http.Get(p.url)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("URL", p.url).With("stack", stack.Trace().TrimRuntime())
	}
	defer resp.Body.Close()

	parser := expfmt.TextParser{}
	metrics, errGo = parser.TextToMetricFamilies(resp.Body)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("URL", p.url).With("stack", stack.Trace().TrimRuntime())
	}
	for k := range metrics {
		if len(prefix) != 0 && !strings.HasPrefix(k, prefix) {
			delete(metrics, k)
		}
	}
	return metrics, nil
}

func (p *prometheusClient) getMetric(prefix string) (items []string, err kv.Error) {

	resp, errGo := http.Get(p.url)
	if errGo != nil {
		return items, kv.Wrap(errGo).With("URL", p.url).With("stack", stack.Trace().TrimRuntime())
	}
	defer resp.Body.Close()

	body, errGo := ioutil.ReadAll(resp.Body)
	if errGo != nil {
		return items, kv.Wrap(errGo).With("URL", p.url).With("stack", stack.Trace().TrimRuntime())
	}

	lines := strings.Split(string(body), "\n")
	for _, line := range lines {
		if len(prefix) == 0 || strings.HasPrefix(line, prefix) {
			items = append(items, line)
		}
	}
	return items, nil
}

func (p *prometheusClient) GetHitsMisses(hash string) (hits int, misses int, err kv.Error) {
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
