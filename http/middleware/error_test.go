package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	apperr "github.com/mehrdad-masoumi/go-packages/errors"
)

func TestErrorHandlerMapsConflict(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	ErrorHandler(apperr.New("test").WithKind(apperr.KindConflict).WithMessage("already exists"), c)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestErrorHandlerHidesUnexpectedCause(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	err := apperr.New("db").WithKind(apperr.KindUnexpected).WithErr(errors.New("password=secret"))
	ErrorHandler(err, c)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "password=secret") {
		t.Fatalf("leaked cause: %s", rec.Body.String())
	}
}
