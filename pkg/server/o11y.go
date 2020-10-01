package server

// This file contains an open telemetry based exporter for the
// honeycomb obswrability service

import (
	"context"
	"os"

	"github.com/leaf-ai/studio-go-runner/pkg/network"

	"github.com/honeycombio/opentelemetry-exporter-go/honeycomb"

	"go.opentelemetry.io/otel/api/global"
	"go.opentelemetry.io/otel/label"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

var (
	hostKey  = label.Key("studio.ml/host")
	nodeKey  = label.Key("studio.ml/node")
	hostName = network.GetHostName()
)

func init() {
	// If the hosts FQDN or network name is not known use the
	// hostname reported by the Kernel
	if hostName == "localhost" || hostName == "unknown" || len(hostName) == 0 {
		hostName, _ = os.Hostname()
	}
}

func StartTelemetry(ctx context.Context, nodeName string, serviceName string, apiKey string, dataset string) (err kv.Error) {

	hny, errGo := honeycomb.NewExporter(
		honeycomb.Config{
			APIKey: apiKey,
		},
		honeycomb.TargetingDataset(dataset),
		honeycomb.WithServiceName(serviceName),
		honeycomb.WithDebugEnabled(),
	)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	tp, errGo := sdktrace.NewProvider(
		sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithSyncer(hny),
	)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	global.SetTraceProvider(tp)

	_, span := global.Tracer(serviceName).Start(ctx, "test-run")
	span.SetAttributes(hostKey.String(hostName))
	if len(nodeName) != 0 {
		span.SetAttributes(nodeKey.String(nodeName))
	}

	go func() {
		<-ctx.Done()

		span.End()
		hny.Close()
	}()

	return nil
}
