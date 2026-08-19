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

func SetOutboxExhausted(service, destination, eventType string, n float64) {
	ensure()
	outboxExhausted.WithLabelValues(SanitizeService(service), sanitizeOutboxDest(destination), SanitizeEventType(eventType)).Set(n)
}

func RecordReconciliationMismatch(service, operation string) {
	ensure()
	reconMismatch.WithLabelValues(SanitizeService(service), SanitizeOperation(operation)).Inc()
}

func RecordReconciliationRepaired(service, operation string) {
	ensure()
	reconRepaired.WithLabelValues(SanitizeService(service), SanitizeOperation(operation)).Inc()
}

// RecordReconciliationScanned counts one item examined by a reconciliation
// pass, regardless of outcome. Use alongside RecordReconciliationRepaired
// and RecordReconciliationFailed to derive pass coverage and success rate.
func RecordReconciliationScanned(service, operation string) {
	ensure()
	reconScanned.WithLabelValues(SanitizeService(service), SanitizeOperation(operation)).Inc()
}

// RecordReconciliationFailed counts a reconciliation item that could not be
// resolved automatically (distinct from RecordReconciliationMismatch, which
// counts detected discrepancies before an attempt to fix them).
func RecordReconciliationFailed(service, operation string) {
	ensure()
	reconFailed.WithLabelValues(SanitizeService(service), SanitizeOperation(operation)).Inc()
}

// RecordAmbiguousOperation counts a financial operation whose downstream
// outcome could not be determined (e.g. a call timed out after the remote
// side may have already committed). Callers must keep the operation pending
// for reconciliation rather than guessing success or failure.
func RecordAmbiguousOperation(service, operation string) {
	ensure()
	ambiguousOps.WithLabelValues(SanitizeService(service), SanitizeOperation(operation)).Inc()
}

func SetCircuitBreakerState(service, destination, state string) {
	ensure()
	var v float64
	switch SanitizeResult(state) {
	case ResultHalfOpen:
		v = 1
	case ResultOpen:
		v = 2
	default:
		v = 0
	}
	circuitBreaker.WithLabelValues(SanitizeService(service), SanitizeService(destination)).Set(v)
}

func RecordClientFailure(source, destination, protocol, errorType string) {
	ensure()
	clientFailures.WithLabelValues(SanitizeService(source), SanitizeService(destination), protocol, SanitizeErrorType(errorType)).Inc()
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
