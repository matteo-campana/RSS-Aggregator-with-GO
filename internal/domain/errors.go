package domain

import (
	"errors"
	"fmt"
)

// The error taxonomy every layer speaks.
//
// Adapters translate their technology-specific failures into these sentinels
// (see storage/postgres.translate), and the HTTP layer maps them to status
// codes. That indirection is what keeps pgx out of the transport package.
var (
	ErrNotFound     = errors.New("resource not found")
	ErrConflict     = errors.New("resource already exists")
	ErrInvalidInput = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
)

// ValidationError names the field that failed validation. It unwraps to
// ErrInvalidInput so callers can use a single errors.Is check.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid input: %s %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error { return ErrInvalidInput }

// NewValidationError builds a ValidationError for the given field.
func NewValidationError(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}
