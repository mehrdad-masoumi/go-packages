package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestJSONSchemaAndContextEnrichment(t *testing.T) {
	var buf bytes.Buffer
	Init(Config{
		Service:     "notification-service",
		Environment: "development",
		Component:   "worker",
		Level:       "debug",
		Output:      &buf,
	})

	tid, _ := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	sid, _ := trace.SpanIDFromHex("0123456789abcdef")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)
	ctx = ContextWithRequestID(ctx, "req_123")

	Info(ctx, "withdrawal created", "error_code", "OK")

	line := buf.String()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &m); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, line)
	}
	for _, k := range []string{"timestamp", "level", "service", "environment", "message", "request_id", "trace_id", "span_id", "component"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing field %q in %s", k, line)
		}
	}
	if m["service"] != "notification-service" {
		t.Fatalf("service=%v", m["service"])
	}
	if m["request_id"] != "req_123" {
		t.Fatalf("request_id=%v", m["request_id"])
	}
	if m["trace_id"] != tid.String() {
		t.Fatalf("trace_id=%v", m["trace_id"])
	}
	if m["level"] != "info" {
		t.Fatalf("level=%v", m["level"])
	}
}
