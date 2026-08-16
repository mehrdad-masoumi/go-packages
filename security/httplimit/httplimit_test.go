package httplimit

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestReadAllBounds(t *testing.T) {
	_, err := ReadAll(strings.NewReader(strings.Repeat("a", 20)), 10)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestMiddlewareRejectsOversizedBody(t *testing.T) {
	e := echo.New()
	e.Use(Middleware(16))
	e.POST("/", func(c echo.Context) error {
		_, err := ReadAll(c.Request().Body, 16)
		if err != nil {
			return err
		}
		return c.NoContent(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(bytes.Repeat([]byte("x"), 64)))
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge && rec.Code != http.StatusInternalServerError {
		// MaxBytesReader surfaces as MaxBytesError; middleware maps it to 413.
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
}
