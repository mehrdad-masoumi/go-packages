package tracing

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// EchoMiddleware returns Echo middleware that extracts W3C context and creates a SERVER span.
func EchoMiddleware(serviceName string) echo.MiddlewareFunc {
	if serviceName == "" {
		serviceName = "http"
	}
	return otelecho.Middleware(serviceName)
}

// HTTPClient wraps an http.Client Transport with OpenTelemetry (injects W3C headers on outbound calls).
func HTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	rt := base.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &http.Client{
		Transport: otelhttp.NewTransport(
			rt,
			otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		),
		CheckRedirect: base.CheckRedirect,
		Jar:           base.Jar,
		Timeout:       base.Timeout,
	}
}

// InjectHTTP injects the current span context into HTTP headers.
func InjectHTTP(ctx context.Context, header http.Header) {
	Propagator().Inject(ctx, propagation.HeaderCarrier(header))
}

// ExtractHTTP extracts span context from HTTP headers into ctx.
func ExtractHTTP(ctx context.Context, header http.Header) context.Context {
	return Propagator().Extract(ctx, propagation.HeaderCarrier(header))
}

// MarkHTTPStatus sets span status from an HTTP status code.
func MarkHTTPStatus(span trace.Span, status int) {
	if span == nil {
		return
	}
	span.SetAttributes(semconv.HTTPResponseStatusCode(status))
	if status >= 500 {
		span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
	}
}

// HTTPRouteAttrs returns low-cardinality attributes for an Echo route.
func HTTPRouteAttrs(c echo.Context) []attribute.KeyValue {
	req := c.Request()
	return []attribute.KeyValue{
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.HTTPRoute(c.Path()),
		semconv.URLPath(req.URL.Path),
	}
}

// SpanFromContext returns the current span (may be a no-op span).
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}
