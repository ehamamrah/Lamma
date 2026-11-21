// Package telemetry configures OpenTelemetry tracing. Export is enabled only
// when an OTLP endpoint is configured; otherwise tracing is a no-op.
package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Shutdown flushes and stops the tracer provider.
type Shutdown func(context.Context) error

// Setup installs a global tracer provider exporting to endpoint (host:port).
func Setup(ctx context.Context, endpoint, version string) (Shutdown, error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}
	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName("lamma-api"), semconv.ServiceVersion(version))
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(0.1))),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
