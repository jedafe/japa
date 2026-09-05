package templates

import "errors"

// Exported var values.
var (
	// ErrTemplateNotFound is a sentinel error: email template not found.
	ErrTemplateNotFound = errors.New("email template not found")
	ErrInvalidType      = errors.New("invalid email template type")
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)
