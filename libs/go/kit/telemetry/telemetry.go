// Package telemetry wires the OpenTelemetry SDK per design section 7: OTLP over
// HTTP/protobuf to OTEL_EXPORTER_OTLP_ENDPOINT, W3C trace context, and a
// resource carrying service.namespace=encabulator.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is unset, Setup installs the propagator and
// nothing else, so tracing is a genuine no-op in tests and local runs.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.38.0"
)

// Namespace is the service.namespace attribute shared by every service.
const Namespace = "encabulator"

// Config describes the tracing setup for one service.
type Config struct {
	// ServiceName is OTEL_SERVICE_NAME.
	ServiceName string
	// ServiceVersion is reported as service.version.
	ServiceVersion string
	// Endpoint is OTEL_EXPORTER_OTLP_ENDPOINT. Empty disables tracing.
	Endpoint string
}

// Shutdown flushes and stops the tracing pipeline. It is safe to call on a
// no-op setup.
type Shutdown func(context.Context) error

// Setup installs the global tracer provider and propagator. The returned
// Shutdown should be deferred by the caller.
func Setup(ctx context.Context, cfg Config) (Shutdown, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	if cfg.Endpoint == "" {
		// The global default provider is already a no-op; leave it in place.
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(traceURL(cfg.Endpoint)))
	if err != nil {
		return nil, fmt.Errorf("telemetry: otlp exporter: %w", err)
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
		semconv.ServiceNamespace(Namespace),
	}
	if cfg.ServiceVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(cfg.ServiceVersion))
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(semconv.SchemaURL, attrs...))
	if err != nil {
		return nil, fmt.Errorf("telemetry: resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(2*time.Second)),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return func(ctx context.Context) error { return tp.Shutdown(ctx) }, nil
}

// traceURL turns a collector base endpoint into the traces signal URL.
// otlptracehttp.WithEndpointURL expects the full path.
func traceURL(endpoint string) string {
	const suffix = "/v1/traces"
	for len(endpoint) > 0 && endpoint[len(endpoint)-1] == '/' {
		endpoint = endpoint[:len(endpoint)-1]
	}
	if len(endpoint) >= len(suffix) && endpoint[len(endpoint)-len(suffix):] == suffix {
		return endpoint
	}
	return endpoint + suffix
}
