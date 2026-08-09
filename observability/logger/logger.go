package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Config configures the process-wide structured logger.
type Config struct {
	Service     string // e.g. notification-service
	Environment string // e.g. development, production
	Component   string // e.g. api, worker, outbox (optional)
	Level       string // debug, info, warn, error (default info)
	Output      io.Writer
}

var (
	mu     sync.RWMutex
	base   *slog.Logger
	cfg    Config
	inited bool
)

// Init sets the global JSON logger. Safe to call once at process start.
func Init(c Config) {
	mu.Lock()
	defer mu.Unlock()

	if c.Output == nil {
		c.Output = os.Stdout
	}
	if c.Level == "" {
		c.Level = "info"
	}
	if c.Service == "" {
		c.Service = envOr("SERVICE_NAME", "unknown-service")
	}
	if c.Environment == "" {
		c.Environment = envOr("ENVIRONMENT", envOr("APP_ENV", "development"))
	}
	if c.Component == "" {
		c.Component = os.Getenv("COMPONENT")
	}

	handler := slog.NewJSONHandler(c.Output, &slog.HandlerOptions{
		Level: parseLevel(c.Level),
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				return slog.Attr{Key: "timestamp", Value: a.Value}
			case slog.MessageKey:
				return slog.Attr{Key: "message", Value: a.Value}
			case slog.LevelKey:
				return slog.Attr{Key: "level", Value: slog.StringValue(strings.ToLower(a.Value.String()))}
			default:
				return a
			}
		},
	})

	attrs := []any{
		"service", c.Service,
		"environment", c.Environment,
	}
	if c.Component != "" {
		attrs = append(attrs, "component", c.Component)
	}

	base = slog.New(handler).With(attrs...)
	cfg = c
	inited = true
	slog.SetDefault(base)
}

// L returns the process logger (without request/trace enrichment).
func L() *slog.Logger {
	mu.RLock()
	if inited && base != nil {
		l := base
		mu.RUnlock()
		return l
	}
	mu.RUnlock()
	// Lazy default so packages can log before Init in tests.
	Init(Config{})
	mu.RLock()
	defer mu.RUnlock()
	return base
}

// FromContext returns a logger enriched with request_id / trace_id / span_id from ctx.
func FromContext(ctx context.Context) *slog.Logger {
	l := L()
	if ctx == nil {
		return l
	}
	attrs := contextAttrs(ctx)
	if len(attrs) == 0 {
		return l
	}
	return l.With(attrs...)
}

// Debug logs at debug level with context enrichment.
func Debug(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Debug(msg, args...)
}

// Info logs at info level with context enrichment.
func Info(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Info(msg, args...)
}

// Warn logs at warn level with context enrichment.
func Warn(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Warn(msg, args...)
}

// Error logs at error level with context enrichment.
func Error(ctx context.Context, msg string, args ...any) {
	FromContext(ctx).Error(msg, args...)
}

// With returns a child context carrying extra slog attributes for subsequent logs.
func With(ctx context.Context, args ...any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) == 0 {
		return ctx
	}
	existing, _ := ctx.Value(extraAttrsKey{}).([]any)
	merged := append(append([]any{}, existing...), args...)
	return context.WithValue(ctx, extraAttrsKey{}, merged)
}

// IsInitialized reports whether Init has been called.
func IsInitialized() bool {
	mu.RLock()
	defer mu.RUnlock()
	return inited
}

// Service returns the configured service name.
func Service() string {
	mu.RLock()
	defer mu.RUnlock()
	return cfg.Service
}

// Environment returns the configured environment name.
func Environment() string {
	mu.RLock()
	defer mu.RUnlock()
	return cfg.Environment
}

// Component returns the configured component name.
func Component() string {
	mu.RLock()
	defer mu.RUnlock()
	return cfg.Component
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

type extraAttrsKey struct{}
