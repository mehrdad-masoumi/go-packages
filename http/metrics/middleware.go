package metrics

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	apperr "github.com/mehrdad-masoumi/go-packages/errors"
	obsmetrics "github.com/mehrdad-masoumi/go-packages/observability/metrics"
)

var (
	httpOnce     sync.Once
	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec
)

func ensureHTTP() {
	httpOnce.Do(func() {
		httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "http_server_requests_total",
			Help: "Inbound HTTP requests by service, method, route pattern, and status class. Probes are excluded.",
		}, []string{"service", "method", "route", "status_class"})

		httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_server_request_duration_seconds",
			Help:    "Inbound HTTP request duration in seconds. Probes are excluded.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"service", "method", "route"})
	})
}

// Middleware records shared RED metrics (http_server_requests_total /
// http_server_request_duration_seconds). Probe and metrics endpoints are skipped
// so they never enter traffic or error-rate calculations.
func Middleware(service string) echo.MiddlewareFunc {
	ensureHTTP()
	svc := obsmetrics.SanitizeService(service)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := c.Request().URL.Path
			if obsmetrics.IsInfraHTTPPath(path) {
				return next(c)
			}
			started := time.Now()
			err := next(c)
			status := c.Response().Status
			if err != nil {
				status = statusFromHandlerError(err, status)
			}
			if status == 0 {
				status = http.StatusOK
			}
			route := boundedRoute(c)
			class := statusClass(status)
			httpRequests.WithLabelValues(svc, c.Request().Method, route, class).Inc()
			httpDuration.WithLabelValues(svc, c.Request().Method, route).Observe(time.Since(started).Seconds())
			return err
		}
	}
}

func boundedRoute(c echo.Context) string {
	route := c.Path()
	if route == "" {
		route = obsmetrics.NormalizePath(c.Request().URL.Path)
	}
	if route == "" {
		return "unmatched"
	}
	if len(route) > 96 {
		return route[:96]
	}
	return route
}

func statusClass(code int) string {
	switch {
	case code >= 500:
		return "5xx"
	case code >= 400:
		return "4xx"
	case code >= 300:
		return "3xx"
	default:
		return "2xx"
	}
}

func statusFromHandlerError(err error, current int) int {
	var ve *apperr.Error
	if errors.As(err, &ve) {
		return http.StatusUnprocessableEntity
	}
	var re *apperr.RichError
	if errors.As(err, &re) && re != nil {
		switch re.Kind() {
		case apperr.KindInvalid:
			return http.StatusBadRequest
		case apperr.KindForbidden:
			return http.StatusForbidden
		case apperr.KindNotFound:
			return http.StatusNotFound
		case apperr.KindUnauthenticated:
			return http.StatusUnauthorized
		case apperr.KindTooManyRequests:
			return http.StatusTooManyRequests
		default:
			return http.StatusInternalServerError
		}
	}
	var he *echo.HTTPError
	if errors.As(err, &he) && he != nil && he.Code > 0 {
		return he.Code
	}
	if current >= 400 {
		return current
	}
	if strings.Contains(strings.ToLower(err.Error()), "validation") {
		return http.StatusUnprocessableEntity
	}
	return http.StatusInternalServerError
}

// StatusClassLabel is exported for tests.
func StatusClassLabel(code int) string { return statusClass(code) }

// MustAtoi is a tiny helper kept for tests that assert status class mapping.
func MustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
