package outbox

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	base, max := time.Second, 5*time.Minute
	if got := Backoff(1, base, max); got != base {
		t.Fatalf("got %v", got)
	}
	if got := Backoff(3, base, max); got != 4*time.Second {
		t.Fatalf("got %v", got)
	}
	if got := Backoff(20, base, max); got != max {
		t.Fatalf("got %v", got)
	}
}
