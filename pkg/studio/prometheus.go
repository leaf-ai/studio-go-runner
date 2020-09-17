// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package studio

// This file contains the implementation of a set of functions that will on a
// regular basis output information about the runner that could be useful to observers

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// prometheusPort is a singleton that contains the port number of the local prometheus server
	// that can be scraped by monitoring tools and the like.
	prometheusPort = int(0) // Stores the dynamically assigned port number used by the prometheus source

	resourceMonitor sync.Once
)

// Allows testing software to query which port is being used by the prometheus metrics server resident
// inside the current server process
func GetPrometheusPort() (port int) {
	return prometheusPort
}

// StartPrometheusExporter loops doing prometheus exports for resource consumption statistics etc
// on a regular basis
func StartPrometheusExporter(ctx context.Context, promAddr string, getRsc ResourceAvailable, update time.Duration, logger *Logger) {

	go monitoringExporter(ctx, getRsc, update, logger)

	// start the prometheus http server for metrics
	go func() {
		if err := runPrometheus(ctx, promAddr, logger); err != nil {
			logger.Warn(fmt.Sprint(err, stack.Trace().TrimRuntime()))
		}
	}()

}

func runPrometheus(ctx context.Context, promAddr string, logger *Logger) (err kv.Error) {
	if len(promAddr) == 0 {
		return nil
	}

	// Allocate a port if none specified, by first checking for a 0 port
	host, port, errGo := net.SplitHostPort(promAddr)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	prometheusPort, errGo = strconv.Atoi(port)
	if errGo != nil {
		return kv.Wrap(errGo, "badly formatted port number for prometheus server").With("port", prometheusPort).With("stack", stack.Trace().TrimRuntime())
	}
	if prometheusPort == 0 {
		prometheusPort, errGo = GetFreePort(promAddr)
		if errGo != nil {
			return kv.Wrap(errGo, "could not allocate listening port for prometheus server").With("address", promAddr).With("stack", stack.Trace().TrimRuntime())
		}
	}

	// Start a monitoring go routine that will gather stats and update the gages and other prometheus
	// collection items

	// The Handler function provides a default handler to expose metrics
	// via an HTTP server. "/metrics" is the usual endpoint for that.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	h := http.Server{
		Addr:    fmt.Sprintf("%s:%d", host, prometheusPort),
		Handler: mux,
	}

	go func() {
		logger.Info(fmt.Sprintf("prometheus listening on %s", h.Addr), "stack", stack.Trace().TrimRuntime())

		logger.Warn(fmt.Sprint(h.ListenAndServe(), "stack", stack.Trace().TrimRuntime()))
	}()

	go func() {
		<-ctx.Done()
		if err := h.Shutdown(context.Background()); err != nil {
			logger.Warn(fmt.Sprint("stopping due to signal", err), "stack", stack.Trace().TrimRuntime())
		}
	}()

	return nil
}

type ResourceAvailable interface {
	FetchMachineResources() (rsc *Resource)
}

// monitoringExporter on a regular basis will invoke prometheus exporters inside our system
//
func monitoringExporter(ctx context.Context, getRsc ResourceAvailable, refreshInterval time.Duration, logger *Logger) {

	lastRefresh := time.Now()

	resourceMonitor.Do(func() {
		refresh := time.NewTicker(30 * time.Second)
		defer refresh.Stop()

		lastMsg := ""
		for {
			select {
			case <-refresh.C:
				msg := getRsc.FetchMachineResources().String()
				if lastMsg != msg || time.Since(lastRefresh) > time.Duration(20*time.Minute) {
					lastRefresh = time.Now()
					logger.Info("capacity", "available", msg)
					lastMsg = msg
				}
			case <-ctx.Done():
				return
			}
		}
	})

	refresh := time.NewTicker(refreshInterval)
	defer refresh.Stop()

	for {
		select {
		case <-refresh.C:
			// The function will update our resource consumption gauges for the
			// host we are running on
			updateGauges(getRsc.FetchMachineResources())

		case <-ctx.Done():
			return
		}
	}
}
