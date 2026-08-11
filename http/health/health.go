package health

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

type Checker interface {
	Ping(ctx context.Context) error
}

type NamedChecker struct {
	Name    string
	Checker Checker
}

type Config struct {
	ReadinessTimeout time.Duration
	LivenessPath     string
	ReadinessPath    string
}

func Register(e *echo.Echo, cfg Config, deps ...NamedChecker) {
	if cfg.ReadinessTimeout <= 0 {
		cfg.ReadinessTimeout = 2 * time.Second
	}
	if cfg.LivenessPath == "" {
		cfg.LivenessPath = "/health-check"
	}
	if cfg.ReadinessPath == "" {
		cfg.ReadinessPath = "/ready"
	}
	e.GET(cfg.LivenessPath, func(c echo.Context) error { return c.JSON(http.StatusOK, map[string]string{"status": "ok"}) })
	e.HEAD(cfg.LivenessPath, func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET(cfg.ReadinessPath, func(c echo.Context) error {
		for _, dep := range deps {
			if dep.Checker == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(c.Request().Context(), cfg.ReadinessTimeout)
			err := dep.Checker.Ping(ctx)
			cancel()
			if err != nil {
				name := dep.Name
				if name == "" {
					name = "dependency"
				}
				return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": name})
			}
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})
}
