package apperr

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/mehrdad-masoumi/go-packages/observability/logger"
)

type errorBody struct {
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// Handler maps apperr types (and Echo HTTPError) to JSON responses.
func Handler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	var ve *Error
	if errors.As(err, &ve) {
		status := http.StatusUnprocessableEntity
		logHTTPError(c, status, err)
		_ = c.JSON(status, errorBody{
			Message: "validation failed",
			Fields:  ve.Fields,
		})
		return
	}

	var re *RichError
	if errors.As(err, &re) {
		status := statusFromKind(re.Kind())
		logHTTPError(c, status, err)
		_ = c.JSON(status, errorBody{Message: re.Message()})
		return
	}

	if he, ok := err.(*echo.HTTPError); ok {
		msg := "error"
		if m, ok := he.Message.(string); ok {
			msg = m
		}
		logHTTPError(c, he.Code, err)
		_ = c.JSON(he.Code, errorBody{Message: msg})
		return
	}

	logHTTPError(c, http.StatusInternalServerError, err)
	_ = c.JSON(http.StatusInternalServerError, errorBody{Message: "internal server error"})
}

func logHTTPError(c echo.Context, status int, err error) {
	// Access log already carries 4xx; only surface causes that need ops attention.
	if status < http.StatusInternalServerError || err == nil {
		return
	}

	req := c.Request()
	ctx := req.Context()
	args := []any{
		"method", req.Method,
		"path", req.URL.Path,
		"status", status,
		"error_code", Code(err),
		"error", err.Error(),
	}

	var re *RichError
	if errors.As(err, &re) {
		args = append(args, "op", re.Op(), "kind", re.Kind().String())
	}
	logger.Error(ctx, "request failed", args...)
}

// Code returns a stable machine-readable error_code for logs/metrics.
func Code(err error) string {
	if err == nil {
		return ""
	}
	var ve *Error
	if errors.As(err, &ve) && ve != nil {
		return "VALIDATION_FAILED"
	}
	var re *RichError
	if errors.As(err, &re) && re != nil {
		return "APP_" + kindUpper(re.Kind())
	}
	return "UNEXPECTED"
}

func kindUpper(k Kind) string {
	switch k {
	case KindInvalid:
		return "INVALID"
	case KindForbidden:
		return "FORBIDDEN"
	case KindNotFound:
		return "NOT_FOUND"
	case KindUnauthenticated:
		return "UNAUTHENTICATED"
	case KindTooManyRequests:
		return "TOO_MANY_REQUESTS"
	default:
		return "UNEXPECTED"
	}
}

func statusFromKind(k Kind) int {
	switch k {
	case KindInvalid:
		return http.StatusBadRequest
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindUnauthenticated:
		return http.StatusUnauthorized
	case KindTooManyRequests:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}
