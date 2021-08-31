// Copyright 2020-2021 (c) The Go Service Components authors. All rights reserved. Issued under the Apache 2.0 License.

package server // import "github.com/leaf-ai/go-service/pkg/server"

// This file contains an open telemetry based exporter for the
// honeycomb observability service

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/leaf-ai/go-service/pkg/log"
	"github.com/leaf-ai/go-service/pkg/network"

	"google.golang.org/grpc/credentials"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpgrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/semconv"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

var (
	hostKey  = "studio.ml/host"
	nodeKey  = "studio.ml/node"
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

	// Create an OTLP exporter, passing in Honeycomb credentials as environment variables.
	exp, errGo := otlp.NewExporter(
		ctx,
		otlpgrpc.NewDriver(
			otlpgrpc.WithEndpoint("api.honeycomb.io:443"),
			otlpgrpc.WithHeaders(map[string]string{
				"x-honeycomb-team":    apiKey,
				"x-honeycomb-dataset": dataset,
			}),
			otlpgrpc.WithTLSCredentials(credentials.NewClientTLSFromCert(nil, "")),
		),
	)

	if err != nil {
		return ctx, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Create a new tracer provider with a batch span processor and the otlp exporter.
	// Add a resource attribute service.name that identifies the service in the Honeycomb UI.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewWithAttributes(semconv.ServiceNameKey.String(serviceName))),
	)

	// Set the Tracer Provider and the W3C Trace Context propagator as globals
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	)

	labels := []attribute.KeyValue{
		attribute.String(hostKey, hostName),
	}
	if len(nodeName) != 0 {
		labels = append(labels, attribute.String(nodeKey, nodeName))
	}

	ctx, span := otel.Tracer(serviceName).Start(ctx, "test-run")
	span.SetAttributes(labels...)

	go func() {
		<-ctx.Done()

		span.End()

		// Allow other processing to terminate before forcably stopping OpenTelemetry collection
		shutCtx, cancel := context.WithTimeout(context.Background(), time.Duration(10*time.Second))
		defer cancel()

		if errGo := tp.Shutdown(shutCtx); errGo != nil {
			fmt.Println(spew.Sdump(errGo), "stack", stack.Trace().TrimRuntime())
		}
	}()

	return ctx, nil
}
