package apperr

import "fmt"

// ValidationError represents field-level validation failures.
// Transport packages decide the concrete status code/serialization.
type ValidationError struct {
	Fields map[string]string `json:"fields"`
}

// Error is kept as an alias for source compatibility with the old apperr.Error.
type Error = ValidationError

func (e *ValidationError) Error() string {
	if e == nil {
		return "validation error"
	}
	return fmt.Sprintf("validation failed: %v", e.Fields)
}
