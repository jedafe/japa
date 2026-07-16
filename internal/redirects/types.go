package redirects

import "errors"

var (
	ErrRedirectNotFound   = errors.New("redirect not found")
	ErrDuplicateSourcePath = errors.New("source_path already exists for this tenant")
	ErrInvalidHTTPStatus  = errors.New("invalid http_status; must be 301, 302, or 307")
)

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)
