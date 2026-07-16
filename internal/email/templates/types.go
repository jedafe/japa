package templates

import "errors"

var (
	ErrTemplateNotFound = errors.New("email template not found")
	ErrInvalidType      = errors.New("invalid email template type")
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)
