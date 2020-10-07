package server

// This file contains an open telemetry based exporter for the
// honeycomb obswrability service

import (
	"context"
	"os"

	"github.com/leaf-ai/studio-go-runner/pkg/log"
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

func StartTelemetry(ctx context.Context, logger *log.Logger, nodeName string, serviceName string, apiKey string, dataset string) (newCtx context.Context, err kv.Error) {

	opts := []honeycomb.ExporterOption{
		honeycomb.TargetingDataset(dataset),
		honeycomb.WithServiceName(serviceName),
	}
	if logger.IsTrace() {
		opts = append(opts, honeycomb.WithDebugEnabled())
	}
	hny, errGo := honeycomb.NewExporter(
		honeycomb.Config{
			APIKey: apiKey,
		},
		opts...)

	if errGo != nil {
		return ctx, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	tp, errGo := sdktrace.NewProvider(
		sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithSyncer(hny),
	)
	if errGo != nil {
		return ctx, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	global.SetTraceProvider(tp)

	labels := []label.KeyValue{
		hostKey.String(hostName),
	}
	if len(nodeName) != 0 {
		labels = append(labels, nodeKey.String(nodeName))
	}

	ctx, span := global.Tracer(serviceName).Start(ctx, "test-run")
	span.SetAttributes(labels...)

	go func() {
		<-ctx.Done()

		span.End()
		hny.Close()
	}()

	return ctx, nil
}
