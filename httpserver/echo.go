package httpserver

import (
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/mehrdad-masoumi/go-packages/apperr"
	"github.com/mehrdad-masoumi/go-packages/observability/logger"
)

// EchoOptions configures shared Echo middleware.
type EchoOptions struct {
	// DisableAccessLog skips structured HTTP access logging.
	DisableAccessLog bool
}

// NewEcho creates an Echo instance with shared middleware and error handler.
// It installs RequestID + structured JSON access logs (stdout) via observability/logger.
func NewEcho() *echo.Echo {
	return NewEchoWithOptions(EchoOptions{})
}

// NewEchoWithOptions creates Echo with optional middleware tweaks.
func NewEchoWithOptions(opts EchoOptions) *echo.Echo {
	ensureLoggerFromEnv()

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = apperr.Handler
	e.Use(middleware.Recover())
	e.Use(logger.RequestIDMiddleware())
	if !opts.DisableAccessLog {
		e.Use(logger.HTTPMiddleware())
	}
	return e
}

// NewEchoMinimal creates Echo with error handler only (e.g. worker health port).
func NewEchoMinimal() *echo.Echo {
	ensureLoggerFromEnv()

	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = apperr.Handler
	return e
}

func ensureLoggerFromEnv() {
	if logger.IsInitialized() {
		return
	}
	logger.Init(logger.Config{
		Service:     strings.TrimSpace(os.Getenv("SERVICE_NAME")),
		Environment: firstNonEmpty(os.Getenv("ENVIRONMENT"), os.Getenv("APP_ENV")),
		Component:   strings.TrimSpace(os.Getenv("COMPONENT")),
		Level:       firstNonEmpty(os.Getenv("LOG_LEVEL"), "info"),
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
