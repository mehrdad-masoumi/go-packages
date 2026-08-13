package server

import (
	"errors"
	"strings"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	httpmetrics "github.com/mehrdad-masoumi/go-packages/http/metrics"
	httpmw "github.com/mehrdad-masoumi/go-packages/http/middleware"
	"github.com/mehrdad-masoumi/go-packages/observability/logger"
)

type Config struct {
	ServiceName             string
	Logger                  logger.Config
	DisableAccessLog        bool
	DisableTracing          bool
	Minimal                 bool
	ExtraAccessLogSkipPaths []string
}

func New(cfg Config) (*echo.Echo, error) {
	cfg.ServiceName = strings.TrimSpace(cfg.ServiceName)
	if !cfg.Minimal && cfg.ServiceName == "" {
		return nil, errors.New("http server: service name is required")
	}
	if !logger.IsInitialized() {
		logCfg := cfg.Logger
		if logCfg.Service == "" {
			logCfg.Service = cfg.ServiceName
		}
		logger.Init(logCfg)
	}
	e := echo.New()
	e.HideBanner = true
	e.HTTPErrorHandler = httpmw.ErrorHandler
	if cfg.Minimal {
		return e, nil
	}

	e.Use(echomw.Recover())
	e.Use(httpmetrics.Middleware(cfg.ServiceName))
	e.Use(httpmw.RequestID())
	if !cfg.DisableTracing {
		e.Use(httpmw.Tracing(cfg.ServiceName))
	}
	if !cfg.DisableAccessLog {
		e.Use(httpmw.AccessLog(cfg.ExtraAccessLogSkipPaths...))
	}
	return e, nil
}

func NewMinimal(logCfg logger.Config) (*echo.Echo, error) {
	return New(Config{Logger: logCfg, Minimal: true})
}
