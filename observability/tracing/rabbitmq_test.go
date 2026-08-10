package tracing

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/propagation"
)

func TestAMQPPropagationRoundTrip(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, span := tp.Tracer("test").Start(context.Background(), "producer")
	headers := InjectAMQP(ctx, amqp.Table{})
	span.End()

	if AMQPHeaderCarrier(headers).Get("traceparent") == "" {
		t.Fatal("expected traceparent header after inject")
	}

	extracted := ExtractAMQP(context.Background(), headers)
	_, consumer := StartConsumerSpan(extracted, "ex", "rk", "q", headers)
	consumer.End()

	spans := exporter.GetSpans()
	if len(spans) < 2 {
		t.Fatalf("expected at least 2 spans, got %d", len(spans))
	}
	parent := spans[0].SpanContext.TraceID()
	child := spans[len(spans)-1].SpanContext.TraceID()
	if parent != child {
		t.Fatalf("trace ids differ: parent=%s child=%s", parent, child)
	}
}
