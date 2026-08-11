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
	KindConflict
	KindUnavailable
)

type RichError struct {
	operation    string
	wrappedError error
	message      string
	kind         Kind
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

func (r *RichError) Message() string {
	if r == nil {
		return "internal server error"
	}
	if r.message != "" {
		return r.message
	}
	var inner *RichError
	if errors.As(r.wrappedError, &inner) {
		return inner.Message()
	}
	switch r.Kind() {
	case KindInvalid:
		return "invalid request"
	case KindForbidden:
		return "forbidden"
	case KindNotFound:
		return "not found"
	case KindUnauthenticated:
		return "unauthenticated"
	case KindTooManyRequests:
		return "too many requests"
	case KindConflict:
		return "conflict"
	case KindUnavailable:
		return "service unavailable"
	default:
		return "internal server error"
	}
}

func (r *RichError) Error() string {
	if r == nil {
		return "nil richerror"
	}
	var out string
	if r.operation != "" {
		out = r.operation
	}
	if r.message != "" {
		if out != "" {
			out += ": "
		}
		out += r.message
	}
	if r.wrappedError != nil {
		if out != "" {
			return out + ": " + r.wrappedError.Error()
		}
		return r.wrappedError.Error()
	}
	if out != "" {
		return out
	}
	return "unknown error"
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
	case KindConflict:
		return "conflict"
	case KindUnavailable:
		return "unavailable"
	default:
		return fmt.Sprintf("kind_%d", int(k))
	}
}

func Code(err error) string {
	if err == nil {
		return ""
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return "VALIDATION_FAILED"
	}
	var re *RichError
	if errors.As(err, &re) {
		switch re.Kind() {
		case KindInvalid:
			return "APP_INVALID"
		case KindForbidden:
			return "APP_FORBIDDEN"
		case KindNotFound:
			return "APP_NOT_FOUND"
		case KindUnexpected:
			return "APP_UNEXPECTED"
		case KindUnauthenticated:
			return "APP_UNAUTHENTICATED"
		case KindTooManyRequests:
			return "APP_TOO_MANY_REQUESTS"
		case KindConflict:
			return "APP_CONFLICT"
		case KindUnavailable:
			return "APP_UNAVAILABLE"
		}
	}
	return "UNEXPECTED"
}
