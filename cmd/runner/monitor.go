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
		logger.Info(fmt.Sprintf("prometheus listening on %s", h.Addr))

		logger.Warn(fmt.Sprint(h.ListenAndServe(), stack.Trace().TrimRuntime()))
	}()

	go func() {
		select {
		case <-ctx.Done():
			if err := h.Shutdown(context.Background()); err != nil {
				logger.Warn(fmt.Sprint("stopping due to signal", err), stack.Trace().TrimRuntime())
			}
		}
	}()

	return nil
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
