package metrics

import (
	"strings"
	"time"
)

func sanitizeOutboxDest(v string) string {
	if s := SanitizeService(v); s != "unknown" {
		return s
	}
	return SanitizeExchange(v)
}

func SetOutboxPending(service, destination, eventType string, n float64) {
	ensure()
	outboxPending.WithLabelValues(SanitizeService(service), sanitizeOutboxDest(destination), SanitizeEventType(eventType)).Set(n)
}

func SetOutboxFailed(service, destination, eventType string, n float64) {
	ensure()
	outboxFailed.WithLabelValues(SanitizeService(service), sanitizeOutboxDest(destination), SanitizeEventType(eventType)).Set(n)
}

func SetOutboxOldestPending(service, destination, eventType string, age time.Duration) {
	ensure()
	if age < 0 {
		age = 0
	}
	outboxOldest.WithLabelValues(SanitizeService(service), sanitizeOutboxDest(destination), SanitizeEventType(eventType)).Set(age.Seconds())
}

func RecordOutboxDispatch(service, destination, eventType, result string, started time.Time) {
	ensure()
	svc := SanitizeService(service)
	dst := sanitizeOutboxDest(destination)
	ev := SanitizeEventType(eventType)
	res := SanitizeResult(result)
	outboxDispatch.WithLabelValues(svc, dst, ev, res).Inc()
	if res == ResultFailed || res == ResultError {
		outboxDispFail.WithLabelValues(svc, dst, ev).Inc()
	}
	if !started.IsZero() && (res == ResultDispatched || res == ResultSuccess || res == ResultFailed || res == ResultError) {
		outboxDispDur.WithLabelValues(svc, dst, ev).Observe(time.Since(started).Seconds())
	}
}

func RecordBusiness(service, operation, result string) {
	ensure()
	op := SanitizeBusinessOp(operation)
	if op == "" {
		return
	}
	svc := SanitizeService(service)
	res := stringsToBusinessResult(result)
	businessTotal.WithLabelValues(svc, op, res).Inc()
	if res == BusinessSystemFailure {
		businessFails.WithLabelValues(svc, op).Inc()
	}
}

func stringsToBusinessResult(result string) string {
	switch strings.ToLower(strings.TrimSpace(result)) {
	case BusinessSuccess, "ok":
		return BusinessSuccess
	case BusinessRejected, "invalid", "rejected":
		return BusinessRejected
	default:
		return BusinessSystemFailure
	}
}
