package rabbitmq

import (
	"context"
	"fmt"

	"github.com/mehrdad-masoumi/go-packages/observability/tracing"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type headerCarrier amqp.Table

func (c headerCarrier) Get(key string) string {
	value, ok := c[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}
func (c headerCarrier) Set(key, value string) { c[key] = value }
func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}
	return keys
}

func InjectTrace(ctx context.Context, headers amqp.Table) amqp.Table {
	if headers == nil {
		headers = amqp.Table{}
	}
	tracing.Propagator().Inject(ctx, headerCarrier(headers))
	return headers
}

func ExtractTrace(ctx context.Context, headers amqp.Table) context.Context {
	if headers == nil {
		return ctx
	}
	return tracing.Propagator().Extract(ctx, headerCarrier(headers))
}

func startProducerSpan(ctx context.Context, exchange, routingKey string, headers amqp.Table) (context.Context, oteltrace.Span, amqp.Table) {
	ctx, span := tracing.Tracer("github.com/mehrdad-masoumi/go-packages/messaging/rabbitmq").Start(ctx, producerSpanName(exchange, routingKey),
		oteltrace.WithSpanKind(oteltrace.SpanKindProducer),
		oteltrace.WithAttributes(messageAttrs(exchange, routingKey, "publish")...),
	)
	return ctx, span, InjectTrace(ctx, headers)
}

func StartConsumerSpan(ctx context.Context, exchange, routingKey, queue string, headers amqp.Table) (context.Context, oteltrace.Span) {
	ctx = ExtractTrace(ctx, headers)
	attrs := messageAttrs(exchange, routingKey, "process")
	if queue != "" {
		attrs = append(attrs, attribute.String("messaging.destination.name", queue))
	}
	return tracing.Tracer("github.com/mehrdad-masoumi/go-packages/messaging/rabbitmq").Start(ctx, consumerSpanName(exchange, routingKey),
		oteltrace.WithSpanKind(oteltrace.SpanKindConsumer), oteltrace.WithAttributes(attrs...))
}

func endSpan(span oteltrace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

func messageAttrs(exchange, routingKey, operation string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{semconv.MessagingSystemRabbitmq, attribute.String("messaging.operation", operation)}
	if exchange != "" {
		attrs = append(attrs, attribute.String("messaging.destination.name", exchange))
	}
	if routingKey != "" {
		attrs = append(attrs, attribute.String("messaging.rabbitmq.destination.routing_key", routingKey))
	}
	return attrs
}
func producerSpanName(exchange, routingKey string) string {
	if routingKey != "" {
		return "publish " + routingKey
	}
	if exchange != "" {
		return "publish " + exchange
	}
	return "publish"
}
func consumerSpanName(exchange, routingKey string) string {
	if routingKey != "" {
		return "process " + routingKey
	}
	if exchange != "" {
		return "process " + exchange
	}
	return "process"
}
