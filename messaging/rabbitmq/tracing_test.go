package rabbitmq

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracePropagationRoundTrip(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx, producer := tp.Tracer("test").Start(context.Background(), "producer")
	headers := InjectTrace(ctx, amqp.Table{})
	producer.End()
	if headers["traceparent"] == nil {
		t.Fatal("traceparent missing")
	}

	ctx, consumer := StartConsumerSpan(context.Background(), "ex", "rk", "q", headers)
	_ = ctx
	consumer.End()
	spans := exporter.GetSpans()
	if len(spans) < 2 {
		t.Fatalf("spans=%d", len(spans))
	}
	if spans[0].SpanContext.TraceID() != spans[len(spans)-1].SpanContext.TraceID() {
		t.Fatal("trace IDs differ")
	}
}
