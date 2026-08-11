package tracing

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

// MapCarrier is a string-map TextMapCarrier useful for message/outbox metadata.
type MapCarrier map[string]string

func (c MapCarrier) Get(key string) string {
	if c == nil {
		return ""
	}
	return c[key]
}

func (c MapCarrier) Set(key, value string) {
	if c != nil {
		c[key] = value
	}
}

func (c MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func InjectMap(ctx context.Context) map[string]string {
	carrier := MapCarrier{}
	Propagator().Inject(ctx, carrier)
	return carrier
}

func ExtractMap(ctx context.Context, carrier map[string]string) context.Context {
	if len(carrier) == 0 {
		return ctx
	}
	return Propagator().Extract(ctx, MapCarrier(carrier))
}

func TraceIDHex(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}
