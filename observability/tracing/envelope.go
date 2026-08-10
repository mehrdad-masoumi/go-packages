package tracing

import (
	"context"
	"encoding/json"
)

type envelopeTrace struct {
	TraceParent string `json:"traceparent"`
	TraceState  string `json:"tracestate"`
}

// ExtractFromJSONCarrier restores W3C context from JSON that embeds
// top-level "traceparent" / "tracestate" fields (e.g. event envelopes).
func ExtractFromJSONCarrier(ctx context.Context, body []byte) context.Context {
	if len(body) == 0 {
		return ctx
	}
	var meta envelopeTrace
	if err := json.Unmarshal(body, &meta); err != nil {
		return ctx
	}
	if meta.TraceParent == "" {
		return ctx
	}
	return ExtractMap(ctx, map[string]string{
		"traceparent": meta.TraceParent,
		"tracestate":  meta.TraceState,
	})
}

// InjectIntoJSONCarrier merges W3C fields into a JSON object body.
// If body is not a JSON object, it is returned unchanged.
func InjectIntoJSONCarrier(ctx context.Context, body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	carrier := InjectMap(ctx)
	if tp := carrier["traceparent"]; tp != "" {
		raw, _ := json.Marshal(tp)
		obj["traceparent"] = raw
	}
	if ts := carrier["tracestate"]; ts != "" {
		raw, _ := json.Marshal(ts)
		obj["tracestate"] = raw
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}
