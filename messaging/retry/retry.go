// Package retry implements bounded RabbitMQ retry without hot requeue.
// Callers must never Nack(requeue=true) for application failures.
package retry

import (
	"strconv"

	amqp "github.com/rabbitmq/amqp091-go"
)

const Header = "x-retry-count"

type Action string

const (
	Ack   Action = "ack"
	Retry Action = "retry"
	DLQ   Action = "dlq"
)

type Decision struct {
	Action  Action
	Attempt int
	Reason  string
}

func Attempt(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	raw, ok := headers[Header]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

// Decide never returns a hot-requeue action. Retriable failures republish
// with an incremented count; exhausted or terminal failures go to DLQ.
func Decide(retriable bool, headers amqp.Table, maxAttempts int) Decision {
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	n := Attempt(headers)
	if !retriable {
		return Decision{Action: DLQ, Attempt: n, Reason: "terminal"}
	}
	if n+1 > maxAttempts {
		return Decision{Action: DLQ, Attempt: n, Reason: "retries_exhausted"}
	}
	return Decision{Action: Retry, Attempt: n + 1, Reason: "retriable"}
}

func WithAttempt(headers amqp.Table, n int) amqp.Table {
	out := amqp.Table{}
	for k, v := range headers {
		out[k] = v
	}
	out[Header] = n
	return out
}
