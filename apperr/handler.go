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

	rid := c.Response().Header().Get(echo.HeaderXRequestID)
	req := c.Request()

	var re *RichError
	if errors.As(err, &re) {
		cause := ""
		if w := re.Unwrap(); w != nil {
			cause = w.Error()
		}
		c.Logger().Errorf(
			"request failed id=%s method=%s uri=%s status=%d op=%s kind=%s message=%q cause=%q",
			rid, req.Method, req.RequestURI, status, re.Op(), re.Kind().String(), re.Message(), cause,
		)
		return
	}

	c.Logger().Errorf(
		"request failed id=%s method=%s uri=%s status=%d err=%v",
		rid, req.Method, req.RequestURI, status, err,
	)
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
