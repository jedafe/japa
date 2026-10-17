package httpx

import (
	"net/url"
	"strings"
)

// IsAbsoluteHTTPURL reports whether raw is an absolute http or https URL
// with a non-empty host. User-supplied link fields (evidence URLs, website
// URLs, booking URLs) that the frontend renders as <a href> MUST pass this
// check before storage — javascript:, data:, vbscript:, and
// protocol-relative inputs are stored-XSS vectors.
//
// url.Parse normalizes the scheme to lowercase, so "JaVaScRiPt:" inputs are
// rejected alongside their lowercase forms. Whitespace-only and oversized
// values are rejected; an empty input returns false — callers decide
// whether the field is optional.
func IsAbsoluteHTTPURL(raw string) bool {
	v := strings.TrimSpace(raw)
	if v == "" || len(v) > 2048 {
		return false
	}
	u, err := url.Parse(v)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return u.Host != ""
}
