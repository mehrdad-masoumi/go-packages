package httplimit

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

var ErrTooLarge = errors.New("payload too large")

const DefaultMaxBody = 1 << 20 // 1 MiB for JSON APIs
const DefaultMaxResponse = 1 << 20

// MaxBytes wraps r.Body so the application cannot read more than n bytes.
func MaxBytes(r *http.Request, n int64) {
	if r == nil || r.Body == nil || n <= 0 {
		return
	}
	r.Body = http.MaxBytesReader(nil, r.Body, n)
}

func Middleware(n int64) echo.MiddlewareFunc {
	if n <= 0 {
		n = DefaultMaxBody
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if req := c.Request(); req != nil && req.Body != nil {
				req.Body = http.MaxBytesReader(c.Response(), req.Body, n)
			}
			err := next(c)
			if err != nil && isMaxBytes(err) {
				return echo.NewHTTPError(http.StatusRequestEntityTooLarge, "payload too large")
			}
			return err
		}
	}
}

func isMaxBytes(err error) bool {
	var mb *http.MaxBytesError
	return errors.As(err, &mb)
}

func ReadAll(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		max = DefaultMaxResponse
	}
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%w: exceeded %d bytes", ErrTooLarge, max)
	}
	return b, nil
}
