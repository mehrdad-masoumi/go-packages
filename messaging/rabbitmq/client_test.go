package rabbitmq

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestWaitStopsOnCancel(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	start := time.Now()
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()
	err := Wait(ctx, time.Hour)
	if err == nil {
		t.Fatal("expected context error")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("wait ignored cancellation: %s", time.Since(start))
	}
}

func TestWaitCompletes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	if err := Wait(ctx, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func newTestClient(t *testing.T, delay time.Duration) *Client {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		cfg:    Config{ReconnectDelay: delay},
		ctx:    ctx,
		cancel: cancel,
	}
	return c
}

func TestCloseDoesNotReconnect(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, time.Hour)
	var dials atomic.Int32
	c.reconnect = func() error {
		dials.Add(1)
		return errorsForTest("dial")
	}
	c.startReconnectLoop()

	done := make(chan struct{})
	go func() {
		_ = c.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() blocked waiting for reconnect backoff")
	}
	if n := dials.Load(); n != 0 {
		t.Fatalf("Close triggered reconnect attempts: %d", n)
	}
	if n := c.reconnects.Load(); n != 0 {
		t.Fatalf("reconnect counter=%d after intentional close", n)
	}
}

func TestCloseStopsReconnectLoopImmediately(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, time.Hour)
	closeCh := make(chan *amqp.Error, 1)
	c.notifyClose = func() <-chan *amqp.Error { return closeCh }
	c.reconnect = func() error { return nil }
	c.startReconnectLoop()

	start := time.Now()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("Close did not join reconnect loop promptly")
	}

	before := c.reconnects.Load()
	closeCh <- amqp.ErrClosed
	time.Sleep(50 * time.Millisecond)
	if c.reconnects.Load() != before {
		t.Fatal("intentional Close() still triggered reconnect")
	}
}

func TestUnexpectedDisconnectReconnects(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, time.Millisecond)
	closeCh := make(chan *amqp.Error, 1)
	c.notifyClose = func() <-chan *amqp.Error { return closeCh }
	reconnected := make(chan struct{}, 1)
	c.reconnect = func() error {
		select {
		case reconnected <- struct{}{}:
		default:
		}
		return nil
	}
	c.startReconnectLoop()

	closeCh <- &amqp.Error{Code: 320, Reason: "CONNECTION_FORCED"}
	select {
	case <-reconnected:
	case <-time.After(2 * time.Second):
		t.Fatal("unexpected disconnect did not reconnect")
	}

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReconnectBackoffStopsOnCancel(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, time.Hour)
	c.notifyClose = func() <-chan *amqp.Error {
		ch := make(chan *amqp.Error, 1)
		ch <- amqp.ErrClosed
		return ch
	}
	started := make(chan struct{})
	c.reconnect = func() error {
		close(started)
		return errorsForTest("broker down")
	}
	c.startReconnectLoop()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("reconnect never attempted")
	}

	start := time.Now()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("reconnect backoff ignored cancellation")
	}
}

func errorsForTest(msg string) error {
	return errString(msg)
}

type errString string

func (e errString) Error() string { return string(e) }
