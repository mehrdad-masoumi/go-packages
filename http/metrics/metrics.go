package metrics

import (
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var registerCollectorsOnce sync.Once

func RegisterCollectors() {
	registerCollectorsOnce.Do(func() {
		_ = prometheus.Register(collectors.NewGoCollector())
		_ = prometheus.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	})
}

func Register(e *echo.Echo, path string) {
	if path == "" {
		path = "/metrics"
	}
	RegisterCollectors()
	e.GET(path, echo.WrapHandler(promhttp.Handler()))
}
