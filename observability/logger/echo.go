package logger

import (
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// RequestIDMiddleware ensures X-Request-ID is present and stored on the request context
// for automatic log enrichment. Reuses Echo's RequestID generator.
func RequestIDMiddleware() echo.MiddlewareFunc {
	rid := middleware.RequestID()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return rid(func(c echo.Context) error {
			id := c.Response().Header().Get(echo.HeaderXRequestID)
			if id == "" {
				id = c.Request().Header.Get(echo.HeaderXRequestID)
			}
			ctx := ContextWithRequestID(c.Request().Context(), id)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		})
	}
}

// skipAccessLogPaths are probe/infra routes that should not emit access logs.
var skipAccessLogPaths = map[string]struct{}{
	"/health-check": {},
	"/ready":        {},
}

// HTTPMiddleware writes one structured JSON access log per request.
// Skips Authorization, cookies, and bodies by design.
// Also skips /health-check and /ready to avoid probe noise.
func HTTPMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			req := c.Request()
			res := c.Response()

			if _, skip := skipAccessLogPaths[req.URL.Path]; skip {
				return err
			}

			status := res.Status
			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok && he.Code > 0 {
					status = he.Code
				} else if status == 0 || status == 200 {
					status = 500
				}
			}

			ctx := req.Context()
			latency := time.Since(start)
			args := []any{
				"method", req.Method,
				"path", req.URL.Path,
				"status", status,
				"duration_ms", latency.Milliseconds(),
				"duration", latency.String(),
			}
			if err != nil && status >= 500 {
				args = append(args,
					"error_code", "HTTP_REQUEST_FAILED",
					"error", err.Error(),
				)
				Error(ctx, "http request failed", args...)
			} else if status >= 500 {
				Error(ctx, "http request failed", args...)
			} else if status >= 400 {
				Warn(ctx, "http request completed", args...)
			} else {
				Info(ctx, "http request completed", args...)
			}
			return err
		}
	}
}

// StatusString is a tiny helper for tests / formatting.
func StatusString(status int) string {
	return strconv.Itoa(status)
}
