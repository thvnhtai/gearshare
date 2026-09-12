// Package observability implements the three pillars the requirement
// checklist names separately but which work together in practice:
// instrumentation (otel.go — traces/metrics generation), monitoring
// (metrics.go — the Prometheus scrape endpoint), and telemetry (this file —
// structured logs correlated to the traces via a shared trace/request ID).
package observability

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// NewLogger returns a structured JSON logger to stdout only — no log
// files, no rotation, per the twelve-factor "logs as event streams"
// requirement (docs/twelve-factor.md factor XI). A real deployment ships
// stdout to whatever log aggregator it uses; the app itself never manages
// log files.
func NewLogger(serviceName string) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler).With("service", serviceName)
}

// WithTrace attaches the active span's trace ID and span ID to a logger, so
// a single booking's log lines are grep-able by trace_id across every
// service boundary it crosses (REST -> gRPC -> Kafka -> consumer) — the
// concrete meaning of "telemetry" in this project's checklist, not just a
// buzzword next to "logging".
func WithTrace(ctx context.Context, logger *slog.Logger) *slog.Logger {
	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()
	if !sc.IsValid() {
		return logger
	}
	return logger.With("trace_id", sc.TraceID().String(), "span_id", sc.SpanID().String())
}
