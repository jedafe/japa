package redirects

import "errors"

// Exported var values.
var (
	// ErrRedirectNotFound is a sentinel error: redirect not found.
	ErrRedirectNotFound    = errors.New("redirect not found")
	ErrDuplicateSourcePath = errors.New("source_path already exists for this platform")
	ErrInvalidHTTPStatus   = errors.New("invalid http_status; must be 301, 302, or 307")
	ErrInvalidTargetURL    = errors.New("target_url must be a valid absolute http/https URL or a root-relative path")
	ErrInvalidSourcePath   = errors.New("source_path must start with /")
)

// Exported const values.
const (
	// DefaultPageSize is an exported constant.
	DefaultPageSize = 20
	MaxPageSize     = 100
)
