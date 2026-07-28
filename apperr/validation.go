package apperr

import "fmt"

// Error represents field-level validation failures (HTTP 422).
type Error struct {
	Fields map[string]string `json:"fields"`
}

func (e *Error) Error() string {
	if e == nil {
		return "validation error"
	}
	return fmt.Sprintf("validation failed: %v", e.Fields)
}
