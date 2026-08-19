package metrics

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mehrdad-masoumi/go-packages/observability/logger"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

type operationKey struct{}

// WithOperation stores a stable logical operation name for metrics and logs.
func WithOperation(ctx context.Context, operation string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	operation = SanitizeOperation(operation)
	ctx = context.WithValue(ctx, operationKey{}, operation)
	return logger.With(ctx, "operation", operation)
}

func OperationFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(operationKey{}).(string)
	return v
}

type httpOperationKey struct{}

// WithHTTPOperation overrides automatic path normalization for one request.
func WithHTTPOperation(ctx context.Context, operation string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, httpOperationKey{}, SanitizeOperation(operation))
}

type eventTypeKey struct{}

func WithEventType(ctx context.Context, eventType string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, eventTypeKey{}, SanitizeEventType(eventType))
}

func EventTypeFromContext(ctx context.Context, fallback string) string {
	if ctx != nil {
		if v, ok := ctx.Value(eventTypeKey{}).(string); ok && v != "" {
			return v
		}
	}
	return SanitizeEventType(fallback)
}

// Transport is an http.RoundTripper that emits unified client RED metrics.
type Transport struct {
	Base        http.RoundTripper
	Source      string
	Destination string
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ensure()
	if req == nil {
		return nil, errors.New("metrics transport: request is nil")
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	source := SanitizeService(t.Source)
	dest := SanitizeService(t.Destination)
	op := httpOpFromRequest(req)
	ctx := WithOperation(req.Context(), op)
	req = req.WithContext(ctx)

	start := time.Now()
	resp, err := base.RoundTrip(req)
	dur := time.Since(start).Seconds()
	status, errType := classifyHTTP(resp, err)
	clientRequests.WithLabelValues(source, dest, ProtocolHTTP, op, status, errType).Inc()
	clientDuration.WithLabelValues(source, dest, ProtocolHTTP, op).Observe(dur)
	if status != StatusSuccess {
		clientFailures.WithLabelValues(source, dest, ProtocolHTTP, errType).Inc()
	}
	return resp, err
}

func httpOpFromRequest(req *http.Request) string {
	if req == nil {
		return "unknown"
	}
	if v, ok := req.Context().Value(httpOperationKey{}).(string); ok && v != "" {
		return v
	}
	path := req.URL.Path
	if path == "" && req.URL != nil {
		path = req.URL.String()
	}
	return HTTPOperation(req.Method, path)
}

func classifyHTTP(resp *http.Response, err error) (status, errorType string) {
	if err != nil {
		return StatusError, errorTypeFromErr(err)
	}
	if resp == nil {
		return StatusError, ErrorUnknown
	}
	return HTTPStatusClass(resp.StatusCode)
}

func errorTypeFromErr(err error) string {
	if err == nil {
		return ErrorNone
	}
	if errors.Is(err, context.Canceled) {
		return ErrorCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorTimeout
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return ErrorTimeout
	case strings.Contains(msg, "canceled") || strings.Contains(msg, "cancelled"):
		return ErrorCanceled
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "connection reset") || strings.Contains(msg, "eof"):
		return ErrorUnavailable
	default:
		return ErrorUnavailable
	}
}

// HTTPClient returns a client that records unified dependency metrics and
// propagates W3C trace context. Timeout and cancellation on base are preserved.
func HTTPClient(base *http.Client, source, destination string) *http.Client {
	if base == nil {
		base = &http.Client{}
	}
	rt := base.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	otelRT := otelhttp.NewTransport(rt, otelhttp.WithPropagators(otel.GetTextMapPropagator()))
	return &http.Client{
		Transport: Transport{
			Base:        otelRT,
			Source:      source,
			Destination: destination,
		},
		CheckRedirect: base.CheckRedirect,
		Jar:           base.Jar,
		Timeout:       base.Timeout,
	}
}
