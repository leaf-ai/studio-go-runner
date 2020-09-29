package server

// This file contains an open telemetry based exporter for the
// honeycomb obswrability service

import (
	"context"

	"github.com/honeycombio/opentelemetry-exporter-go/honeycomb"

	"go.opentelemetry.io/otel/api/global"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

func StartTelemetry(ctx context.Context, serviceName string, apiKey string, dataset string) (err kv.Error) {

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

	_, span := global.Tracer(serviceName).Start(ctx, "test")

	go func() {
		<-ctx.Done()

		span.End()
		hny.Close()
	}()

	return nil
}
