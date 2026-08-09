package httpserver

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/mehrdad-masoumi/go-packages/apperr"
)

// NewEcho creates an Echo instance with shared middleware and error handler.
func NewEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = apperr.Handler
	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: `{"time":"${time_rfc3339}","id":"${id}","method":"${method}","uri":"${uri}","status":${status},"latency":"${latency_human}","error":"${error}"}` + "\n",
	}))
	return e
}

// NewEchoMinimal creates Echo with error handler only (e.g. worker health port).
func NewEchoMinimal() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = apperr.Handler
	return e
}
