package apperr

import (
	"errors"
	"testing"
)

func TestRichErrorMessageHidesUnexpectedCause(t *testing.T) {
	err := New("user.Login").
		WithKind(KindUnexpected).
		WithMessage("failed to issue tokens").
		WithErr(errors.New("dial tcp: connection refused"))
	if got := err.Message(); got != "failed to issue tokens" {
		t.Fatalf("Message() = %q", got)
	}
}

func TestKindPropagatesThroughWrapping(t *testing.T) {
	inner := New("repo.Get").WithKind(KindNotFound).WithMessage("not found")
	outer := New("service.Get").WithErr(inner)
	if got := outer.Kind(); got != KindNotFound {
		t.Fatalf("Kind() = %v", got)
	}
}

func TestCode(t *testing.T) {
	if got := Code(New("x").WithKind(KindConflict)); got != "APP_CONFLICT" {
		t.Fatalf("Code() = %q", got)
	}
}
