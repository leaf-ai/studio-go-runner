package main

// This file containes the implementation of a set of functions that will on a
// regular basis output information about the runner that could be useful to observers

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/SentientTechnologies/studio-go-runner"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	promAddrOpt = flag.String("prom-address", "", "the address for the prometheus http server within the runner")

	cpuTemp = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cpu_temperature_celsius",
		Help: "Current temperature of the CPU.",
	})
)

func init() {
	prometheus.MustRegister(cpuTemp)
}

func runPrometheus(ctx context.Context) {
	if len(*promAddrOpt) == 0 {
		return
	}

	// Start a monitoring go routine that will gather stats and update the gages and other prometheus
	// collection items

	// The Handler function provides a default handler to expose metrics
	// via an HTTP server. "/metrics" is the usual endpoint for that.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	h := http.Server{Addr: *promAddrOpt, Handler: mux}

	go logger.Warn(fmt.Sprintf("%#v", h.ListenAndServe()))

	h.Shutdown(ctx)
}

func showResources(ctx context.Context) {

	res := &runner.Resources{}

	refresh := time.NewTicker(5 * time.Second)
	defer refresh.Stop()

	showTime := time.NewTicker(5 * time.Minute)
	defer showTime.Stop()

	lastMsg := ""
	nextOutput := time.Now()

	for {
		select {
		case <-refresh.C:
			if msg := res.Dump(); msg != lastMsg {
				logger.Info("dump resources " + msg)
				lastMsg = msg
				nextOutput = time.Now().Add(time.Duration(5 * time.Minute))
			}
		case <-showTime.C:
			if !time.Now().Before(nextOutput) {
				lastMsg = res.Dump()
				logger.Info("dump resources " + lastMsg)
				nextOutput = time.Now().Add(time.Duration(5 * time.Minute))
			}

		case <-ctx.Done():
			return
		}
	}
}
