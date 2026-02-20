package service

import "errors"

// Sentinel errors for service-level operations
var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("conflict")
	ErrInsufficientFunds  = errors.New("insufficient funds")
)

// ValidationError represents a validation error with additional context
type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "validation error"
}
