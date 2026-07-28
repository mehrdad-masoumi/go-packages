package httpserver

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

func RegisterHealth(e *echo.Echo, deps ...NamedChecker) {
	e.GET("/health-check", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})
	e.HEAD("/health-check", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.GET("/ready", func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		for _, dep := range deps {
			if dep.Checker == nil {
				continue
			}
			if err := dep.Checker.Ping(ctx); err != nil {
				name := dep.Name
				if name == "" {
					name = "dependency"
				}
				return c.JSON(http.StatusServiceUnavailable, map[string]string{
					"status": "not_ready",
					"error":  name,
				})
			}
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ready"})
	})
}
