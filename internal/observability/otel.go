package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"github.com/thvnhtai/gearshare/internal/config"
)

// SetupTracing wires OpenTelemetry's SDK to export spans to a local
// Jaeger/Tempo collector over OTLP-HTTP (docker-compose.yml's `jaeger`
// service). Called once per process (cmd/api, cmd/notification-service,
// cmd/search-indexer each call this with their own service name), so a
// trace started by a REST handler and continued by a gRPC call, a Kafka
// producer/consumer hop, or a RabbitMQ publish/consume all land in the same
// trace view — see internal/middleware (REST), the gRPC interceptors, and
// eventbus/queue's header/property propagation for how the trace context
// actually crosses each boundary.
func SetupTracing(ctx context.Context, cfg config.OTelConfig) (func(context.Context) error, error) {
	if !cfg.Enabled {
		otel.SetTracerProvider(sdktrace.NewTracerProvider()) // no-op provider
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create otlp exporter: %w", err)
	}

	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(cfg.ServiceName),
	))
	if err != nil {
		return nil, fmt.Errorf("observability: build resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}
