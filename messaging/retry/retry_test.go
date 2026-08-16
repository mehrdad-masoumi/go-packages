package retry

import (
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestDecideNeverHotRequeues(t *testing.T) {
	d := Decide(true, nil, 3)
	if d.Action != Retry {
		t.Fatalf("got %s", d.Action)
	}
	d = Decide(true, amqp.Table{Header: 3}, 3)
	if d.Action != DLQ {
		t.Fatalf("exhausted must DLQ, got %s", d.Action)
	}
	d = Decide(false, nil, 8)
	if d.Action != DLQ {
		t.Fatalf("terminal must DLQ, got %s", d.Action)
	}
}
