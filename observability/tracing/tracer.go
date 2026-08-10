package tracing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const instrumentationName = "github.com/mehrdad-masoumi/go-packages/observability/tracing"

var (
	mu       sync.Mutex
	provider *sdktrace.TracerProvider
)

// Config controls tracer initialization.
// Prefer standard OTEL_* environment variables; Config fields override when non-empty.
type Config struct {
	// Enabled defaults to true when OTEL_SDK_DISABLED is not "true".
	// Set explicitly to false to disable.
	Enabled *bool

	// ServiceName maps to service.name (also OTEL_SERVICE_NAME).
	ServiceName string

	// Endpoint is host:port or URL for OTLP gRPC (also OTEL_EXPORTER_OTLP_ENDPOINT).
	Endpoint string

	// Environment maps to deployment.environment.
	Environment string

	// ServiceVersion maps to service.version.
	ServiceVersion string

	// InstanceID maps to service.instance.id (defaults to hostname).
	InstanceID string

	// SampleRatio is used with parentbased_traceidratio (0.0–1.0).
	// Defaults from OTEL_TRACES_SAMPLER_ARG or 1.0.
	SampleRatio *float64
}

// Init configures the global TracerProvider and W3C TraceContext propagator.
// Returns a shutdown function that flushes remaining spans.
func Init(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	if !isEnabled(cfg) {
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
		return func(context.Context) error { return nil }, nil
	}

	endpoint := firstNonEmpty(cfg.Endpoint, os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), "infra-jaeger:4317")
	endpoint = stripOTLPScheme(endpoint)

	serviceName := firstNonEmpty(cfg.ServiceName, os.Getenv("OTEL_SERVICE_NAME"), os.Getenv("SERVICE_NAME"), "unknown-service")
	environment := firstNonEmpty(cfg.Environment, os.Getenv("ENVIRONMENT"), os.Getenv("APP_ENV"), "development")
	version := firstNonEmpty(cfg.ServiceVersion, os.Getenv("SERVICE_VERSION"), os.Getenv("OTEL_SERVICE_VERSION"))
	instanceID := firstNonEmpty(cfg.InstanceID, os.Getenv("HOSTNAME"))
	if instanceID == "" {
		instanceID, _ = os.Hostname()
	}

	ratio := 1.0
	if cfg.SampleRatio != nil {
		ratio = *cfg.SampleRatio
	} else if v := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG")); v != "" {
		if parsed, err := parseRatio(v); err == nil {
			ratio = parsed
		}
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceName(serviceName),
		semconv.DeploymentEnvironment(environment),
	}
	if version != "" {
		attrs = append(attrs, semconv.ServiceVersion(version))
	}
	if instanceID != "" {
		attrs = append(attrs, semconv.ServiceInstanceID(instanceID))
	}
	attrs = append(attrs, parseResourceAttributes(os.Getenv("OTEL_RESOURCE_ATTRIBUTES"))...)

	res, err := resource.New(ctx, resource.WithAttributes(attrs...))
	if err != nil {
		return nil, fmt.Errorf("tracing resource: %w", err)
	}

	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("tracing otlp dial %s: %w", endpoint, err)
	}

	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("tracing otlp exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)

	mu.Lock()
	if provider != nil {
		mu.Unlock()
		_ = tp.Shutdown(ctx)
		_ = conn.Close()
		return nil, fmt.Errorf("tracing already initialized")
	}
	provider = tp
	mu.Unlock()

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return func(shutdownCtx context.Context) error {
		mu.Lock()
		p := provider
		provider = nil
		mu.Unlock()
		if p == nil {
			return nil
		}
		if shutdownCtx == nil {
			var cancel context.CancelFunc
			shutdownCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
		}
		return p.Shutdown(shutdownCtx)
	}, nil
}

// Tracer returns an instrumented tracer for the shared package name, or service override.
func Tracer(name string) trace.Tracer {
	if name == "" {
		name = instrumentationName
	}
	return otel.Tracer(name)
}

// Start starts a span with optional attributes.
func Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer(instrumentationName).Start(ctx, name, opts...)
}

// RecordError records err on the span and sets status to Error when err != nil.
func RecordError(span trace.Span, err error) {
	if span == nil || err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// Propagator returns the global text-map propagator.
func Propagator() propagation.TextMapPropagator {
	return otel.GetTextMapPropagator()
}

func isEnabled(cfg Config) bool {
	if cfg.Enabled != nil {
		return *cfg.Enabled
	}
	disabled := strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true")
	return !disabled
}

func stripOTLPScheme(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "grpc://")
	return endpoint
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func parseRatio(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func parseResourceAttributes(raw string) []attribute.KeyValue {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []attribute.KeyValue
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		out = append(out, attribute.String(k, v))
	}
	return out
}

// BoolPtr is a helper for Config.Enabled.
func BoolPtr(v bool) *bool { return &v }

// FloatPtr is a helper for Config.SampleRatio.
func FloatPtr(v float64) *float64 { return &v }
