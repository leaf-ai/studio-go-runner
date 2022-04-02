// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.
package main

// This file contains test code for the prometheus metrics collection and
// also with retrieving values

import (
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/andreidenissov-cog/go-service/pkg/network"
	"github.com/andreidenissov-cog/go-service/pkg/server"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prom2json"
	"github.com/rs/xid"
)

var (
	gaugeName = xid.New().String()

	gauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: gaugeName,
			Help: "Number of experiments being actively worked on per queue.",
		},
		[]string{"host", "type", "name", "field_1", "field_2"},
	)

	serialize sync.Mutex
)

func init() {
	prometheus.MustRegister(gauge)
}

func fetchPromCnt(name string) (cnt float64, err kv.Error) {

	port := server.GetPrometheusPort()
	if port == 0 {
		return 0.0, kv.NewError("test failed, prometheus exporter port not found")
	}

	metricsC := make(chan *dto.MetricFamily, 1)
	waiter := make(chan struct{})

	go func() {
		defer close(waiter)
		for {
			metric := <-metricsC
			if metric == nil {
				return
			}
			if metric.GetName() == name {
				for _, m := range metric.Metric {
					cnt += getMetricValue(m)
				}
			}
		}
	}()

	if errGo := prom2json.FetchMetricFamilies("http://localhost:"+strconv.Itoa(port)+"/metrics", metricsC, nil); errGo != nil {
		return 0.0, kv.Wrap(errGo)
	}
	<-waiter
	return cnt, nil
}

func fetchRunnerCnt(name string) (cnt float64, err kv.Error) {
	port := server.GetPrometheusPort()
	if port == 0 {
		return 0.0, kv.NewError("test failed, prometheus exporter port not found")
	}

	pClient := runner.NewPrometheusClient(fmt.Sprintf("http://localhost:%d/metrics", port))
	family, err := pClient.Fetch(name)
	if err != nil {
		return 0, err
	}
	for _, metric := range family {
		for _, m := range metric.Metric {
			cnt += getMetricValue(m)
		}
	}

	return cnt, err
}

// TestPrometheusRaw exercises the go standard library version of the gauge
func TestPrometheusRaw(t *testing.T) {

	labels := prometheus.Labels{
		"host":    network.GetHostName(),
		"type":    xid.New().String(),
		"name":    xid.New().String(),
		"field_1": xid.New().String(),
		"field_2": xid.New().String(),
	}

	// We allow the tests in this file to run in a parallel test environment but
	// we lock to protect their implementation.  This will help in situations where
	// tests have multiple threads available for these very short tests to be interleaved
	// among other tests
	t.Parallel()

	serialize.Lock()
	defer serialize.Unlock()

	startCnt, err := fetchPromCnt(gaugeName)
	if err != nil {
		t.Fatal(err.Error())
	}

	gauge.With(labels).Inc()

	cnt, err := fetchPromCnt(gaugeName)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !almostEqual(startCnt+1.0, cnt) {
		t.Fatal("Retrieved value was ", cnt, " and should have been close to ", startCnt+1.0)
	}

	// After testing the access via the runners API exercise an accumulation function
	// that the runner uses to ensure it returns the same result
	val, err := GetGaugeAccum(gauge)
	if err != nil {
		t.Fatal(err)
	}
	if !almostEqual(val, cnt) {
		t.Fatal("Retrieved value was ", cnt, " and should have been close to ", val)
	}
}

// TestPrometheusRunner exercises the runners own prometheus http version retrieval of the gauge
func TestPrometheusRunner(t *testing.T) {

	labels := prometheus.Labels{
		"host":    network.GetHostName(),
		"type":    xid.New().String(),
		"name":    xid.New().String(),
		"field_1": xid.New().String(),
		"field_2": xid.New().String(),
	}
	// We allow the tests in this file to run in a parallel test environment but
	// we lock to protect their implementation.  This will help in situations where
	// tests have multiple threads available for these very short tests to be interleaved
	// among other tests
	t.Parallel()

	serialize.Lock()
	defer serialize.Unlock()

	startCnt, err := fetchRunnerCnt(gaugeName)
	if err != nil {
		t.Fatal(err.Error())
	}

	gauge.With(labels).Inc()

	cnt, err := fetchRunnerCnt(gaugeName)
	if err != nil {
		t.Fatal(err.Error())
	}

	if !almostEqual(startCnt+1.0, cnt) {
		t.Fatal("Retrieved value was ", cnt, " and should have been close to ", startCnt+1.0)
	}
}
