package tracing

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// StartBusiness starts an INTERNAL span for an important business operation.
func StartBusiness(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	opts = append([]trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindInternal)}, opts...)
	return Tracer(instrumentationName).Start(ctx, name, opts...)
}
