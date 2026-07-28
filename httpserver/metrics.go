package httpserver

import (
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var registerCollectorsOnce sync.Once

// RegisterCollectors registers Go and process collectors once.
func RegisterCollectors() {
	registerCollectorsOnce.Do(func() {
		_ = prometheus.Register(collectors.NewGoCollector())
		_ = prometheus.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	})
}

// RegisterMetrics mounts GET /metrics on the Echo instance.
func RegisterMetrics(e *echo.Echo) {
	RegisterCollectors()
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
}
