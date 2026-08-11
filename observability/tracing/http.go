package tracing

import (
	"context"
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// HTTPClient returns a shallow copy of base with an OTEL-instrumented transport.
// A bounded Timeout should be configured by the caller.
func HTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	rt := base.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &http.Client{
		Transport:     otelhttp.NewTransport(rt, otelhttp.WithPropagators(otel.GetTextMapPropagator())),
		CheckRedirect: base.CheckRedirect,
		Jar:           base.Jar,
		Timeout:       base.Timeout,
	}
}

func InjectHTTP(ctx context.Context, header http.Header) {
	Propagator().Inject(ctx, propagation.HeaderCarrier(header))
}

func ExtractHTTP(ctx context.Context, header http.Header) context.Context {
	return Propagator().Extract(ctx, propagation.HeaderCarrier(header))
}

func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}
