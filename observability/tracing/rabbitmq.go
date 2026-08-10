package tracing

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// AMQPHeaderCarrier adapts amqp.Table to the OpenTelemetry TextMapCarrier interface.
type AMQPHeaderCarrier amqp.Table

// Get returns the value for key.
func (c AMQPHeaderCarrier) Get(key string) string {
	if c == nil {
		return ""
	}
	v, ok := c[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprint(t)
	}
}

// Set stores key/value in the table.
func (c AMQPHeaderCarrier) Set(key, value string) {
	if c == nil {
		return
	}
	c[key] = value
}

// Keys returns all header keys.
func (c AMQPHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectAMQP injects W3C trace context into AMQP headers (creating the table if needed).
func InjectAMQP(ctx context.Context, headers amqp.Table) amqp.Table {
	if headers == nil {
		headers = amqp.Table{}
	}
	Propagator().Inject(ctx, AMQPHeaderCarrier(headers))
	return headers
}

// ExtractAMQP extracts W3C trace context from AMQP headers into ctx.
func ExtractAMQP(ctx context.Context, headers amqp.Table) context.Context {
	if headers == nil {
		return ctx
	}
	return Propagator().Extract(ctx, AMQPHeaderCarrier(headers))
}

// StartProducerSpan creates a PRODUCER span and injects context into headers.
// The returned headers must be attached to the AMQP publishing.
func StartProducerSpan(ctx context.Context, exchange, routingKey string, headers amqp.Table) (context.Context, trace.Span, amqp.Table) {
	ctx, span := Tracer(instrumentationName).Start(ctx, producerSpanName(exchange, routingKey),
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(messagingAttrs(exchange, routingKey, "publish")...),
	)
	headers = InjectAMQP(ctx, headers)
	return ctx, span, headers
}

// StartConsumerSpan extracts context from delivery headers and starts a CONSUMER span.
func StartConsumerSpan(ctx context.Context, exchange, routingKey, queue string, headers amqp.Table) (context.Context, trace.Span) {
	ctx = ExtractAMQP(ctx, headers)
	return Tracer(instrumentationName).Start(ctx, consumerSpanName(exchange, routingKey),
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(messagingAttrs(exchange, routingKey, "process")...),
		trace.WithAttributes(attribute.String("messaging.destination.name", firstNonEmpty(queue, exchange))),
	)
}

// EndSpan ends the span and records err when non-nil.
func EndSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
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

func messagingAttrs(exchange, routingKey, operation string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		semconv.MessagingSystemRabbitmq,
		attribute.String("messaging.operation", operation),
	}
	if exchange != "" {
		attrs = append(attrs, attribute.String("messaging.destination.name", exchange))
	}
	if routingKey != "" {
		attrs = append(attrs, attribute.String("messaging.rabbitmq.destination.routing_key", routingKey))
	}
	return attrs
}

// MapCarrier is a string map TextMapCarrier (useful for outbox / JSON metadata).
type MapCarrier map[string]string

func (c MapCarrier) Get(key string) string {
	if c == nil {
		return ""
	}
	return c[key]
}

func (c MapCarrier) Set(key, value string) {
	if c == nil {
		return
	}
	c[key] = value
}

func (c MapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

// InjectMap returns a map with W3C traceparent/tracestate from ctx.
func InjectMap(ctx context.Context) map[string]string {
	carrier := MapCarrier{}
	Propagator().Inject(ctx, carrier)
	return carrier
}

// ExtractMap restores context from a W3C carrier map.
func ExtractMap(ctx context.Context, carrier map[string]string) context.Context {
	if len(carrier) == 0 {
		return ctx
	}
	return Propagator().Extract(ctx, MapCarrier(carrier))
}

// TraceIDHex returns the hex-encoded trace id from ctx, or empty if invalid.
func TraceIDHex(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}
