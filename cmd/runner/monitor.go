package main

// This file containes the implementation of a set of functions that will on a
// regular basis output information about the runner that could be useful to observers

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/SentientTechnologies/studio-go-runner"
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	promAddrOpt = flag.String("prom-address", ":9090", "the address for the prometheus http server within the runner")

	PrometheusPort = int(0)
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

	PrometheusPort, errGo = strconv.Atoi(port)
	if errGo != nil {
		return errors.Wrap(errGo, "badly formatted port number for prometheus server").With("port", PrometheusPort).With("stack", stack.Trace().TrimRuntime())
	}
	if PrometheusPort == 0 {
		PrometheusPort, errGo = runner.GetFreePort(*promAddrOpt)
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
		Addr:    fmt.Sprintf("%s:%d", host, PrometheusPort),
		Handler: mux,
	}

	go func() {
		logger.Info(fmt.Sprintf("prometheus listening on %s", h.Addr))

		logger.Warn(fmt.Sprintf("%#v", h.ListenAndServe()))
	}()

	h.Shutdown(ctx)

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
