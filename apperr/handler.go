package apperr

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
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
		_ = c.JSON(http.StatusUnprocessableEntity, errorBody{
			Message: "validation failed",
			Fields:  ve.Fields,
		})
		return
	}

	var re *RichError
	if errors.As(err, &re) {
		_ = c.JSON(statusFromKind(re.Kind()), errorBody{Message: re.Error()})
		return
	}

	if he, ok := err.(*echo.HTTPError); ok {
		msg := "error"
		if m, ok := he.Message.(string); ok {
			msg = m
		}
		_ = c.JSON(he.Code, errorBody{Message: msg})
		return
	}

	_ = c.JSON(http.StatusInternalServerError, errorBody{Message: "internal server error"})
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
