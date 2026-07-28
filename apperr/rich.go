package apperr

import "errors"

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

func (r *RichError) Error() string {
	if r == nil {
		return "nil richerror"
	}
	if r.message != "" {
		return r.message
	}
	if r.wrappedError != nil {
		return r.wrappedError.Error()
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
