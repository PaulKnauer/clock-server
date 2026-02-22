package domain

import "fmt"

// ValidationError is returned when domain invariants are violated.
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

// NewValidationError creates a new domain validation error.
func NewValidationError(message string) error {
	return ValidationError{Message: message}
}

// NewValidationErrorf creates a formatted domain validation error.
func NewValidationErrorf(format string, args ...any) error {
	return ValidationError{Message: fmt.Sprintf(format, args...)}
}
