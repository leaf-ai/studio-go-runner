package main

// This file contains the implementation of a set of functions that will on a
// regular basis output information about the runner that could be useful to observers

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	promAddrOpt = flag.String("prom-address", ":9090", "the address for the prometheus http server within the runner")

	// prometheusPort is a singleton that contains the port number of the local prometheus server
	// that can be scraped by monitoring tools and the like.
	prometheusPort = int(0) // Stores the dynamically assigned port number used by the prometheus source
)

func runPrometheus(ctx context.Context) (err errors.Error) {
	if len(*promAddrOpt) == 0 {
		return nil
	}

	// Allocate a port if none specified, by first checking for a 0 port
	host, port, errGo := net.SplitHostPort(*promAddrOpt)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	prometheusPort, errGo = strconv.Atoi(port)
	if errGo != nil {
		return errors.Wrap(errGo, "badly formatted port number for prometheus server").With("port", prometheusPort).With("stack", stack.Trace().TrimRuntime())
	}
	if prometheusPort == 0 {
		prometheusPort, errGo = runner.GetFreePort(*promAddrOpt)
		if errGo != nil {
			return errors.Wrap(errGo, "could not allocate listening port for prometheus server").With("address", *promAddrOpt).With("stack", stack.Trace().TrimRuntime())
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
		select {
		case <-ctx.Done():
			if err := h.Shutdown(context.Background()); err != nil {
				logger.Warn(fmt.Sprint("stopping due to signal", err), "stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	return nil
}

// monitoringExporter on a regular basis will invoke prometheus exporters inside our system
//
func monitoringExporter(ctx context.Context, refreshInterval time.Duration) {

	refresh := time.NewTicker(refreshInterval)
	defer refresh.Stop()

	for {
		select {
		case <-refresh.C:
			// The function will update our resource consumption gauges for the
			// host we are running on
			updateGauges()

		case <-ctx.Done():
			return
		}
	}
}
