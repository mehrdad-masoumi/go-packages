package apperr

import (
	"errors"
	"fmt"
)

type Kind int

const (
	KindInvalid Kind = iota + 1
	KindForbidden
	KindNotFound
	KindUnexpected
	KindUnauthenticated
	KindTooManyRequests
)

type RichError struct {
	operation    string
	wrappedError error
	message      string
	kind         Kind
}

// Message returns the client-facing message (without the wrapped cause).
func (r *RichError) Message() string {
	if r == nil {
		return "nil richerror"
	}
	if r.message != "" {
		return r.message
	}
	if r.wrappedError != nil {
		var inner *RichError
		if errors.As(r.wrappedError, &inner) {
			return inner.Message()
		}
		return r.wrappedError.Error()
	}
	return "unknown error"
}

// Error returns a full chain for logs: op: message: cause.
func (r *RichError) Error() string {
	if r == nil {
		return "nil richerror"
	}

	var b string
	if r.operation != "" {
		b = r.operation
	}
	if r.message != "" {
		if b != "" {
			b += ": "
		}
		b += r.message
	}
	if r.wrappedError != nil {
		cause := r.wrappedError.Error()
		if b != "" {
			return b + ": " + cause
		}
		return cause
	}
	if b != "" {
		return b
	}
	return "unknown error"
}

func New(op string) *RichError {
	return &RichError{operation: op}
}

func (r *RichError) WithErr(err error) *RichError {
	r.wrappedError = err
	return r
}

func (r *RichError) WithMessage(msg string) *RichError {
	r.message = msg
	return r
}

func (r *RichError) WithKind(k Kind) *RichError {
	r.kind = k
	return r
}

func (r *RichError) Kind() Kind {
	if r == nil {
		return KindUnexpected
	}
	if r.kind != 0 {
		return r.kind
	}
	var inner *RichError
	if errors.As(r.wrappedError, &inner) {
		return inner.Kind()
	}
	return KindUnexpected
}

func (r *RichError) Unwrap() error {
	if r == nil {
		return nil
	}
	return r.wrappedError
}

func (r *RichError) Op() string {
	if r == nil {
		return ""
	}
	return r.operation
}

// KindString returns a stable label for logs.
func (k Kind) String() string {
	switch k {
	case KindInvalid:
		return "invalid"
	case KindForbidden:
		return "forbidden"
	case KindNotFound:
		return "not_found"
	case KindUnexpected:
		return "unexpected"
	case KindUnauthenticated:
		return "unauthenticated"
	case KindTooManyRequests:
		return "too_many_requests"
	default:
		return fmt.Sprintf("kind_%d", int(k))
	}
}
