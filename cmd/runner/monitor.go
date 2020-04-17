// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

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
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	promAddrOpt = flag.String("prom-address", ":9090", "the address for the prometheus http server within the runner")

	// prometheusPort is a singleton that contains the port number of the local prometheus server
	// that can be scraped by monitoring tools and the like.
	prometheusPort = int(0) // Stores the dynamically assigned port number used by the prometheus source

	resourceMonitor sync.Once
)

func runPrometheus(ctx context.Context) (err kv.Error) {
	if len(*promAddrOpt) == 0 {
		return nil
	}

	// Allocate a port if none specified, by first checking for a 0 port
	host, port, errGo := net.SplitHostPort(*promAddrOpt)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	prometheusPort, errGo = strconv.Atoi(port)
	if errGo != nil {
		return kv.Wrap(errGo, "badly formatted port number for prometheus server").With("port", prometheusPort).With("stack", stack.Trace().TrimRuntime())
	}
	if prometheusPort == 0 {
		prometheusPort, errGo = runner.GetFreePort(*promAddrOpt)
		if errGo != nil {
			return kv.Wrap(errGo, "could not allocate listening port for prometheus server").With("address", *promAddrOpt).With("stack", stack.Trace().TrimRuntime())
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

// getMachineResources extracts the current system state in terms of memory etc
// and coverts this into the resource specification used by jobs.  Because resources
// specified by users are not exact quantities the resource is used for the machines
// resources even in the face of some loss of precision
//
func getMachineResources() (rsc *runner.Resource) {

	rsc = &runner.Resource{}

	// For specified queue look for any free slots on existing GPUs is
	// applicable and fill them, or find empty GPUs and groups to fill
	// in with work

	cpus, v := runner.CPUFree()
	rsc.Cpus = uint(cpus)
	rsc.Ram = humanize.Bytes(v)

	rsc.Hdd = humanize.Bytes(runner.GetDiskFree())

	// go runner allows GPU resources at the board level so obtain the total slots across
	// all board form factors and use that as our max
	//
	rsc.Gpus = runner.TotalFreeGPUSlots()
	rsc.GpuMem = humanize.Bytes(runner.LargestFreeGPUMem())

	return rsc
}

// monitoringExporter on a regular basis will invoke prometheus exporters inside our system
//
func monitoringExporter(ctx context.Context, refreshInterval time.Duration) {

	resourceMonitor.Do(func() {
		refresh := time.NewTicker(30 * time.Second)
		defer refresh.Stop()

		lastMsg := ""
		for {
			select {
			case <-refresh.C:
				msg := getMachineResources().String()
				if lastMsg != msg {
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
			updateGauges()

		case <-ctx.Done():
			return
		}
	}
}
