package middleware

import (
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

func Tracing(serviceName string) echo.MiddlewareFunc {
	if serviceName == "" {
		serviceName = "http"
	}
	return otelecho.Middleware(serviceName)
}
