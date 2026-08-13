package runtime

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type seqRecorder struct {
	mu   sync.Mutex
	evts []string
}

func (r *seqRecorder) add(s string) {
	r.mu.Lock()
	r.evts = append(r.evts, s)
	r.mu.Unlock()
}

func (r *seqRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.evts))
	copy(out, r.evts)
	return out
}

func TestHTTPGracefulShutdownCompletesInFlightRequest(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{Handler: handler}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	app := New(Config{ShutdownTimeout: 2 * time.Second})
	app.AddHTTP("http", addr, HTTPFromServer(srv))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, dialErr := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never listened: %v", dialErr)
		}
		time.Sleep(10 * time.Millisecond)
	}

	reqErr := make(chan error, 1)
	go func() {
		r, e := http.Get("http://" + addr + "/")
		if e != nil {
			reqErr <- e
			return
		}
		defer r.Body.Close()
		if r.StatusCode != http.StatusOK {
			reqErr <- errors.New(r.Status)
			return
		}
		reqErr <- nil
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request did not start")
	}
	cancel()

	if err := <-reqErr; err != nil {
		t.Fatalf("in-flight request failed: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestContextCancellationStopsApp(t *testing.T) {
	t.Parallel()

	stopped := make(chan struct{})
	app := New(Config{ShutdownTimeout: time.Second})
	app.AddRunner("worker", RunnerFunc(func(ctx context.Context) error {
		<-ctx.Done()
		close(stopped)
		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(ctx) }()

	time.Sleep(30 * time.Millisecond)
	if !app.Ready() {
		t.Fatal("expected ready while running")
	}
	cancel()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if app.Ready() {
		t.Fatal("expected unready after shutdown")
	}
}

func TestWorkerStopsCleanly(t *testing.T) {
	t.Parallel()

	var stopped atomic.Bool
	app := New(Config{ShutdownTimeout: time.Second})
	app.AddWorker("loop", RunnerFunc(func(ctx context.Context) error {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				stopped.Store(true)
				return nil
			case <-ticker.C:
			}
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(ctx) }()
	time.Sleep(40 * time.Millisecond)
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !stopped.Load() {
		t.Fatal("worker did not stop cleanly")
	}
}

func TestCriticalRunnerFailureShutsDown(t *testing.T) {
	t.Parallel()

	fail := errors.New("boom")
	var otherStopped atomic.Bool
	app := New(Config{ShutdownTimeout: time.Second})
	app.AddRunner("ok", RunnerFunc(func(ctx context.Context) error {
		<-ctx.Done()
		otherStopped.Store(true)
		return nil
	}))
	app.AddRunner("bad", RunnerFunc(func(ctx context.Context) error {
		time.Sleep(30 * time.Millisecond)
		return fail
	}))

	err := app.Run(context.Background())
	if !errors.Is(err, fail) {
		t.Fatalf("got %v, want %v", err, fail)
	}
	if !otherStopped.Load() {
		t.Fatal("sibling runner was not cancelled")
	}
}

func TestShutdownDeadlineDoesNotHang(t *testing.T) {
	t.Parallel()

	app := New(Config{ShutdownTimeout: 80 * time.Millisecond})
	app.AddRunner("stuck", RunnerFunc(func(ctx context.Context) error {
		select {}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(ctx) }()
	time.Sleep(20 * time.Millisecond)
	start := time.Now()
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected shutdown deadline error")
		}
		if time.Since(start) > time.Second {
			t.Fatalf("shutdown took too long: %s", time.Since(start))
		}
	case <-time.After(time.Second):
		t.Fatal("application hung on stuck runner")
	}
}

func TestGRPCTimeoutFallsBackToStop(t *testing.T) {
	t.Parallel()

	fake := &hangingGRPC{gracefulDelay: time.Second, stopped: make(chan struct{})}
	app := New(Config{ShutdownTimeout: 50 * time.Millisecond})
	app.AddGRPC("grpc", fake, func() error {
		<-fake.stopped
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("grpc shutdown hung")
	}
	if !fake.stopCalled.Load() {
		t.Fatal("expected Stop fallback after GracefulStop timeout")
	}
}

func TestResourceOrderingClosesAfterRunners(t *testing.T) {
	t.Parallel()

	rec := &seqRecorder{}
	app := New(Config{ShutdownTimeout: time.Second})
	app.AddCloser("db", closerFunc(func() error {
		rec.add("close:db")
		return nil
	}))
	app.AddCloser("rabbit", closerFunc(func() error {
		rec.add("close:rabbit")
		return nil
	}))
	app.AddRunner("consumer", RunnerFunc(func(ctx context.Context) error {
		<-ctx.Done()
		rec.add("stop:consumer")
		return nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}

	got := rec.snapshot()
	want := []string{"stop:consumer", "close:rabbit", "close:db"}
	if len(got) != len(want) {
		t.Fatalf("events=%v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events=%v want %v", got, want)
		}
	}
}

func TestReadinessClearsBeforeShutdown(t *testing.T) {
	t.Parallel()

	sawUnready := make(chan struct{})
	app := New(Config{ShutdownTimeout: time.Second})
	app.AddHTTP("http", "127.0.0.1:0", HTTPFromServer(&http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}))
	app.AddCloseFunc("observe", func(context.Context) error {
		if app.Ready() {
			t.Error("still ready while closing resources")
		} else {
			close(sawUnready)
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- app.Run(ctx) }()
	time.Sleep(30 * time.Millisecond)
	if err := app.Ping(context.Background()); err != nil {
		t.Fatalf("expected ready: %v", err)
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-sawUnready:
	default:
		t.Fatal("readiness was not cleared before closers")
	}
}

type hangingGRPC struct {
	gracefulDelay time.Duration
	stopCalled    atomic.Bool
	stopped       chan struct{}
	once          sync.Once
}

func (h *hangingGRPC) ensure() {
	h.once.Do(func() {
		if h.stopped == nil {
			h.stopped = make(chan struct{})
		}
	})
}

func (h *hangingGRPC) GracefulStop() {
	h.ensure()
	time.Sleep(h.gracefulDelay)
}

func (h *hangingGRPC) Stop() {
	h.ensure()
	h.stopCalled.Store(true)
	select {
	case <-h.stopped:
	default:
		close(h.stopped)
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

var _ io.Closer = closerFunc(func() error { return nil })
