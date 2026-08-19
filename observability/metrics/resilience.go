package metrics

// Dependency-resilience telemetry for outbound service-to-service calls.
//
// Every label here is bounded: source/destination are sanitized service
// names, protocol is http or grpc, error_type is one of the ErrorX
// constants, and result is one of the ResultX constants. Never pass a raw
// error string, URL, user ID, or transaction ID into these helpers.

// RecordClientRetry counts a retry that was actually performed, after the
// retry policy decided the request was replayable.
func RecordClientRetry(source, destination, protocol string) {
	ensure()
	clientRetries.WithLabelValues(
		SanitizeService(source),
		SanitizeService(destination),
		sanitizeProtocol(protocol),
	).Inc()
}

// RecordClientTimeout counts a single attempt that exceeded its deadline.
func RecordClientTimeout(source, destination, protocol string) {
	ensure()
	clientTimeouts.WithLabelValues(
		SanitizeService(source),
		SanitizeService(destination),
		sanitizeProtocol(protocol),
	).Inc()
}

// RecordClientGiveUp counts a dependency call that finally failed: retries
// exhausted, the caller deadline expired, or the breaker rejected it.
func RecordClientGiveUp(source, destination, protocol, errorType string) {
	ensure()
	clientGiveUp.WithLabelValues(
		SanitizeService(source),
		SanitizeService(destination),
		sanitizeProtocol(protocol),
		sanitizeErrorType(errorType),
	).Inc()
}

// RecordCircuitBreakerTransition counts a breaker state change. state must be
// ResultOpen, ResultHalfOpen, or ResultClosed.
func RecordCircuitBreakerTransition(service, destination, state string) {
	ensure()
	circuitBreakerTx.WithLabelValues(
		SanitizeService(service),
		SanitizeService(destination),
		sanitizeBreakerState(state),
	).Inc()
}

func sanitizeProtocol(protocol string) string {
	switch protocol {
	case ProtocolHTTP, ProtocolGRPC:
		return protocol
	default:
		return ProtocolHTTP
	}
}

func sanitizeErrorType(errorType string) string {
	switch errorType {
	case ErrorNone, ErrorUnauthenticated, ErrorForbidden, ErrorNotFound,
		ErrorInvalid, ErrorConflict, ErrorRateLimited, ErrorTimeout,
		ErrorCanceled, ErrorUnavailable, ErrorServer:
		return errorType
	default:
		return ErrorUnknown
	}
}

func sanitizeBreakerState(state string) string {
	switch state {
	case ResultOpen, ResultHalfOpen, ResultClosed:
		return state
	default:
		return ResultClosed
	}
}
