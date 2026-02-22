package application

import "errors"

var (
	// ErrValidation indicates a client-side validation problem.
	ErrValidation = errors.New("validation error")
	// ErrDownstream indicates a downstream transport/integration problem.
	ErrDownstream = errors.New("downstream error")
)
