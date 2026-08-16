// Package outcome classifies remote financial-call results so callers never
// treat transport failures as authoritative business failures.
package outcome

import (
	"errors"
	"net"
	"net/http"
	"strings"
)

type Kind string

const (
	Success         Kind = "success"
	DefiniteFailure Kind = "definite_failure"
	Ambiguous       Kind = "ambiguous_outcome"
)

type Error struct {
	Kind   Kind
	Status int
	Err    error
}

func (e *Error) Error() string {
	if e == nil || e.Err == nil {
		return string(e.kind())
	}
	return e.Err.Error()
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) kind() Kind {
	if e == nil {
		return Success
	}
	return e.Kind
}

func ClassifyHTTP(status int, transportErr error) Kind {
	if transportErr != nil {
		if isDefiniteLocal(transportErr) {
			return DefiniteFailure
		}
		return Ambiguous
	}
	if status >= 200 && status < 300 {
		return Success
	}
	switch status {
	case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusConflict:
		return Ambiguous
	}
	if status >= 500 || status == 0 {
		return Ambiguous
	}
	if status >= 400 {
		return DefiniteFailure
	}
	return Ambiguous
}

func isDefiniteLocal(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "marshal") || strings.Contains(msg, "build request")
}

func IsAmbiguous(err error) bool {
	var oe *Error
	if errors.As(err, &oe) {
		return oe.Kind == Ambiguous
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, p := range []string{"timeout", "deadline", "connection reset", "eof", "broken pipe", "connection refused", "outcome unknown"} {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

func Wrap(kind Kind, status int, err error) error {
	if err == nil {
		return nil
	}
	return &Error{Kind: kind, Status: status, Err: err}
}
