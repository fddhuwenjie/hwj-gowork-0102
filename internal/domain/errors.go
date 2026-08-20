package domain

import "errors"

var (
	ErrValidation        = errors.New("validation failed")
	ErrInvalidTransition = errors.New("invalid transition")
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
)
