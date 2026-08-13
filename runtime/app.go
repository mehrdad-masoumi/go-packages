// Package runtime is a small application lifecycle helper.
//
// It is not a DI framework. Services construct dependencies explicitly and
// register servers, background runners, and closable resources with App.
//
// Shutdown order is deterministic:
//  1. mark the process unready
//  2. stop HTTP/gRPC servers (stop accepting new traffic)
//  3. cancel runner contexts (stop accepting new work)
//  4. wait for runners to drain, up to ShutdownTimeout
//  5. close resources in reverse registration order
package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultShutdownTimeout = 25 * time.Second
	shutdownTimeoutEnv     = "SHUTDOWN_TIMEOUT"
)

var ErrNotReady = errors.New("shutting down")

type Config struct {
	Name            string
	ShutdownTimeout time.Duration
}

type Runner interface {
	Run(context.Context) error
}

type RunnerFunc func(context.Context) error

func (f RunnerFunc) Run(ctx context.Context) error { return f(ctx) }

type HTTPServer interface {
	Start(address string) error
	Shutdown(ctx context.Context) error
}

type GRPCServer interface {
	GracefulStop()
	Stop()
}

type App struct {
	cfg     Config
	ready   atomic.Bool
	servers []namedServer
	runners []namedRunner
	closers []namedCloser
}

type namedServer struct {
	name     string
	serve    func(context.Context) error
	shutdown func(context.Context) error
}

type namedRunner struct {
	name string
	run  func(context.Context) error
}

type namedCloser struct {
	name  string
	close func(context.Context) error
}

func New(cfg Config) *App {
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = timeoutFromEnv()
	}
	return &App{cfg: cfg}
}

func timeoutFromEnv() time.Duration {
	if v := os.Getenv(shutdownTimeoutEnv); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return DefaultShutdownTimeout
}

func (a *App) AddHTTP(name, addr string, srv HTTPServer) {
	if name == "" {
		name = "http"
	}
	a.servers = append(a.servers, namedServer{
		name: name,
		serve: func(context.Context) error {
			err := srv.Start(addr)
			if err == nil || errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		},
		shutdown: srv.Shutdown,
	})
}

func (a *App) AddGRPC(name string, srv GRPCServer, serve func() error) {
	if name == "" {
		name = "grpc"
	}
	a.servers = append(a.servers, namedServer{
		name: name,
		serve: func(context.Context) error {
			if serve == nil {
				return nil
			}
			return serve()
		},
		shutdown: func(ctx context.Context) error {
			return gracefulStopGRPC(ctx, srv)
		},
	})
}

func (a *App) AddServer(name string, serve func(context.Context) error, shutdown func(context.Context) error) {
	if name == "" {
		name = "server"
	}
	a.servers = append(a.servers, namedServer{name: name, serve: serve, shutdown: shutdown})
}

func (a *App) AddRunner(name string, r Runner) {
	if r == nil {
		return
	}
	if name == "" {
		name = "runner"
	}
	a.runners = append(a.runners, namedRunner{name: name, run: r.Run})
}

func (a *App) AddWorker(name string, r Runner) {
	a.AddRunner(name, r)
}

func (a *App) AddConsumer(name string, r Runner) {
	a.AddRunner(name, r)
}

func (a *App) AddCloser(name string, c io.Closer) {
	if c == nil {
		return
	}
	if name == "" {
		name = "closer"
	}
	a.closers = append(a.closers, namedCloser{
		name: name,
		close: func(context.Context) error {
			return c.Close()
		},
	})
}

func (a *App) AddCloseFunc(name string, fn func(context.Context) error) {
	if fn == nil {
		return
	}
	if name == "" {
		name = "closer"
	}
	a.closers = append(a.closers, namedCloser{name: name, close: fn})
}

func (a *App) Ready() bool {
	return a.ready.Load()
}

func (a *App) Ping(context.Context) error {
	if !a.ready.Load() {
		return ErrNotReady
	}
	return nil
}

func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("runtime: context is required")
	}

	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	errCh := make(chan error, len(a.servers)+len(a.runners)+1)
	var wg sync.WaitGroup

	start := func(name string, fn func(context.Context) error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(runCtx); err != nil && runCtx.Err() == nil {
				errCh <- fmt.Errorf("%s: %w", name, err)
				runCancel()
			}
		}()
	}

	for _, s := range a.servers {
		start(s.name, s.serve)
	}
	for _, r := range a.runners {
		start(r.name, r.run)
	}

	a.ready.Store(true)

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-errCh:
		runErr = err
	}

	a.ready.Store(false)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer shutdownCancel()

	var shutdownErr error
	for i := len(a.servers) - 1; i >= 0; i-- {
		s := a.servers[i]
		if s.shutdown == nil {
			continue
		}
		if err := s.shutdown(shutdownCtx); err != nil && shutdownErr == nil {
			shutdownErr = fmt.Errorf("%s shutdown: %w", s.name, err)
		}
	}

	runCancel()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		if shutdownErr == nil {
			shutdownErr = fmt.Errorf("runtime: shutdown deadline exceeded waiting for runners")
		}
	}

	for i := len(a.closers) - 1; i >= 0; i-- {
		c := a.closers[i]
		if err := c.close(shutdownCtx); err != nil && shutdownErr == nil {
			shutdownErr = fmt.Errorf("%s close: %w", c.name, err)
		}
	}

	if runErr != nil {
		return runErr
	}
	return shutdownErr
}

type stdHTTP struct {
	srv *http.Server
}

func HTTPFromServer(srv *http.Server) HTTPServer {
	return stdHTTP{srv: srv}
}

func (s stdHTTP) Start(addr string) error {
	if addr != "" {
		s.srv.Addr = addr
	}
	return s.srv.ListenAndServe()
}

func (s stdHTTP) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func gracefulStopGRPC(ctx context.Context, srv GRPCServer) error {
	if srv == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		srv.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		srv.Stop()
		return ctx.Err()
	}
}
