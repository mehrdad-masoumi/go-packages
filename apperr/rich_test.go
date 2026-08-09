package apperr

import (
	"errors"
	"testing"
)

func TestRichErrorMessageHidesCause(t *testing.T) {
	err := New("user_service.Login").
		WithKind(KindUnexpected).
		WithMessage("failed to issue tokens").
		WithErr(errors.New("dial tcp: connection refused"))

	if got := err.Message(); got != "failed to issue tokens" {
		t.Fatalf("Message() = %q, want client-safe message", got)
	}
}

func TestRichErrorErrorIncludesCause(t *testing.T) {
	err := New("user_service.Login").
		WithKind(KindUnexpected).
		WithMessage("failed to issue tokens").
		WithErr(errors.New("dial tcp: connection refused"))

	got := err.Error()
	want := "user_service.Login: failed to issue tokens: dial tcp: connection refused"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}
