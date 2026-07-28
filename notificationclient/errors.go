package notificationclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors returned by Client implementations. Use errors.Is to
// check for these; use errors.As with *StatusError to inspect the raw HTTP
// status code and any (non-sensitive) server-provided message.
var (
	// ErrConflict means notification-service rejected the request as a
	// duplicate (HTTP 409), typically an idempotency key collision. The
	// original request may already have been accepted; do not blindly
	// retry with a new idempotency key.
	ErrConflict = errors.New("notificationclient: conflict")

	// ErrValidation means the request body failed server-side validation
	// (HTTP 422). Retrying the same payload will not help.
	ErrValidation = errors.New("notificationclient: validation failed")

	// ErrRateLimited means notification-service is throttling the caller
	// (HTTP 429). Callers should back off before retrying.
	ErrRateLimited = errors.New("notificationclient: rate limited")

	// ErrUnavailable means notification-service returned a server error
	// (HTTP 5xx) or the request could not be completed at all (network
	// error, timeout, context cancellation). The outcome of the call is
	// unknown: it may or may not have been processed.
	ErrUnavailable = errors.New("notificationclient: service unavailable")

	// ErrUnauthorized means the internal API key was missing/invalid
	// (HTTP 401/403).
	ErrUnauthorized = errors.New("notificationclient: unauthorized")
)

// StatusError wraps a failed notification-service HTTP call. It satisfies
// errors.Is against the sentinel errors above (via Unwrap) and can be
// retrieved with errors.As for the raw status code.
//
// Message is best-effort and derived only from the server's JSON error
// envelope ({"message": "..."}); it is never the raw response body, and
// this package never logs it.
type StatusError struct {
	StatusCode int
	Message    string

	sentinel error
}

func (e *StatusError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("notificationclient: http %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("notificationclient: http %d", e.StatusCode)
}

// Unwrap lets errors.Is(err, ErrConflict/ErrValidation/...) work against a
// *StatusError.
func (e *StatusError) Unwrap() error {
	return e.sentinel
}

type errorEnvelope struct {
	Message string `json:"message"`
}

// mapStatusError converts a non-success HTTP response into a typed error.
// body is expected to be the (size-limited) response body; only its
// "message" field, if present, is ever surfaced.
func mapStatusError(statusCode int, body []byte) error {
	se := &StatusError{
		StatusCode: statusCode,
		Message:    extractMessage(body),
	}

	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		se.sentinel = ErrUnauthorized
	case statusCode == http.StatusConflict:
		se.sentinel = ErrConflict
	case statusCode == http.StatusUnprocessableEntity:
		se.sentinel = ErrValidation
	case statusCode == http.StatusTooManyRequests:
		se.sentinel = ErrRateLimited
	case statusCode >= http.StatusInternalServerError:
		se.sentinel = ErrUnavailable
	}

	return se
}

func extractMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return ""
	}
	return env.Message
}
