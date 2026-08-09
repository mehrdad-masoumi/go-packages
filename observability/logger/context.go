package logger

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

type requestIDKey struct{}

// ContextWithRequestID stores request_id for automatic log enrichment.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestIDFromContext returns the request_id previously stored on ctx.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(requestIDKey{}).(string)
	return v
}

func contextAttrs(ctx context.Context) []any {
	var attrs []any

	if rid := RequestIDFromContext(ctx); rid != "" {
		attrs = append(attrs, "request_id", rid)
	}

	span := trace.SpanFromContext(ctx)
	sc := span.SpanContext()
	hasOTel := sc.IsValid()
	if hasOTel {
		attrs = append(attrs,
			"trace_id", sc.TraceID().String(),
			"span_id", sc.SpanID().String(),
		)
	}

	if extra, ok := ctx.Value(extraAttrsKey{}).([]any); ok && len(extra) > 0 {
		// Skip duplicate trace_id from message headers when OTel already set it.
		if hasOTel {
			filtered := make([]any, 0, len(extra))
			for i := 0; i+1 < len(extra); i += 2 {
				key, ok := extra[i].(string)
				if ok && key == "trace_id" {
					continue
				}
				filtered = append(filtered, extra[i], extra[i+1])
			}
			attrs = append(attrs, filtered...)
		} else {
			attrs = append(attrs, extra...)
		}
	}

	return attrs
}
