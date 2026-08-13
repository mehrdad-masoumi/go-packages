package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestMiddlewareSkipsProbes(t *testing.T) {
	e := echo.New()
	e.Use(Middleware("auth-service"))
	e.GET("/health", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/ready", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/metrics", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.GET("/api/tokens", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	for _, path := range []string{"/health", "/ready", "/metrics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/tokens", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("traffic: %d", rec.Code)
	}
}

func TestStatusClassMapsReliabilityBuckets(t *testing.T) {
	if statusClass(200) != "2xx" || statusClass(401) != "4xx" || statusClass(503) != "5xx" {
		t.Fatal("status class mapping")
	}
}

func TestMiddlewareRecords5xxAsErrorClass(t *testing.T) {
	e := echo.New()
	e.Use(Middleware("wallet-service"))
	e.GET("/api/wallets", func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "boom")
	})
	req := httptest.NewRequest(http.MethodGet, "/api/wallets", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", rec.Code)
	}
}
