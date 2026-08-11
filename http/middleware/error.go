package middleware

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	apperr "github.com/mehrdad-masoumi/go-packages/errors"
	"github.com/mehrdad-masoumi/go-packages/observability/logger"
)

type ErrorBody struct {
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func ErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	var ve *apperr.ValidationError
	if errors.As(err, &ve) {
		write(c, http.StatusUnprocessableEntity, ErrorBody{Message: "validation failed", Fields: ve.Fields}, err)
		return
	}
	var re *apperr.RichError
	if errors.As(err, &re) {
		write(c, statusFromKind(re.Kind()), ErrorBody{Message: re.Message()}, err)
		return
	}
	if he, ok := err.(*echo.HTTPError); ok {
		msg := "error"
		if value, ok := he.Message.(string); ok {
			msg = value
		}
		write(c, he.Code, ErrorBody{Message: msg}, err)
		return
	}
	write(c, http.StatusInternalServerError, ErrorBody{Message: "internal server error"}, err)
}

func write(c echo.Context, status int, body ErrorBody, err error) {
	if status >= 500 {
		req := c.Request()
		args := []any{"method", req.Method, "path", req.URL.Path, "status", status, "error_code", apperr.Code(err)}
		if err != nil {
			args = append(args, "error", err.Error())
		}
		var re *apperr.RichError
		if errors.As(err, &re) {
			args = append(args, "op", re.Op(), "kind", re.Kind().String())
		}
		logger.Error(req.Context(), "request failed", args...)
	}
	if jsonErr := c.JSON(status, body); jsonErr != nil {
		logger.Error(c.Request().Context(), "write error response failed", "error", jsonErr.Error())
	}
}

func statusFromKind(kind apperr.Kind) int {
	switch kind {
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
	case apperr.KindConflict:
		return http.StatusConflict
	case apperr.KindUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
