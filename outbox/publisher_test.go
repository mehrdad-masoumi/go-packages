package outbox

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	base := time.Second
	max := 5 * time.Minute
	if got := Backoff(1, base, max); got != base {
		t.Fatalf("attempt 1: got %v want %v", got, base)
	}
	if got := Backoff(3, base, max); got != 4*time.Second {
		t.Fatalf("attempt 3: got %v want %v", got, 4*time.Second)
	}
	if got := Backoff(20, base, max); got != max {
		t.Fatalf("capped: got %v want %v", got, max)
	}
}
