package logger

import "context"

// ErrorWithCode logs an error and attaches error_code + error message fields.
func ErrorWithCode(ctx context.Context, msg string, err error, code string, args ...any) {
	fields := make([]any, 0, len(args)+4)
	fields = append(fields, args...)
	if code != "" {
		fields = append(fields, "error_code", code)
	}
	if err != nil {
		fields = append(fields, "error", err.Error())
	}
	Error(ctx, msg, fields...)
}
