package middleware

import (
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/mehrdad-masoumi/go-packages/observability/logger"
)

func RequestID() echo.MiddlewareFunc {
	base := echomw.RequestID()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return base(func(c echo.Context) error {
			id := c.Response().Header().Get(echo.HeaderXRequestID)
			if id == "" {
				id = c.Request().Header.Get(echo.HeaderXRequestID)
			}
			ctx := logger.ContextWithRequestID(c.Request().Context(), id)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		})
	}
}

func AccessLog(skipPaths ...string) echo.MiddlewareFunc {
	skip := map[string]struct{}{"/health-check": {}, "/ready": {}, "/metrics": {}}
	for _, path := range skipPaths {
		skip[path] = struct{}{}
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			req, res := c.Request(), c.Response()
			if _, ok := skip[req.URL.Path]; ok {
				return err
			}
			status := res.Status
			if err != nil {
				if he, ok := err.(*echo.HTTPError); ok && he.Code > 0 {
					status = he.Code
				} else if status == 0 || status == httpStatusOK {
					status = 500
				}
			}
			args := []any{"method", req.Method, "path", req.URL.Path, "status", status, "duration_ms", time.Since(start).Milliseconds()}
			switch {
			case status >= 500:
				if err != nil {
					args = append(args, "error", err.Error())
				}
				logger.Error(req.Context(), "http request completed", args...)
			case status >= 400:
				logger.Warn(req.Context(), "http request completed", args...)
			default:
				logger.Info(req.Context(), "http request completed", args...)
			}
			return err
		}
	}
}

const httpStatusOK = 200
