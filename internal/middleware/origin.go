package middleware

import (
	"os"
	"strings"
)

// IsOriginAllowed reports whether an Origin header value may talk to this API.
// In development, localhost and 127.0.0.1 with any port are allowed. In
// production only origins from JC_CORS_ORIGINS (comma-separated) are
// permitted, falling back to the origins derived from JC_SITE_URL. The
// wildcard "*" is never allowed in production.
//
// It backs both the CORS middleware and the WebSocket upgrade origin check so
// the two can never drift apart.
func IsOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}

	isProd := strings.EqualFold(strings.TrimSpace(os.Getenv("JC_ENV")), "production")
	if !isProd {
		return IsLocalOrigin(origin)
	}

	if origin == "*" {
		return false
	}

	allowedCSV := os.Getenv("JC_CORS_ORIGINS")
	if allowedCSV == "" {
		return isDefaultProductionOrigin(origin, os.Getenv("JC_SITE_URL"))
	}

	for _, allowed := range strings.Split(allowedCSV, ",") {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" || allowed == "*" {
			continue // skip empty entries and never allow wildcard
		}

		if origin == allowed {
			return true
		}
	}

	return false
}

// IsLocalOrigin returns true if the origin is localhost or 127.0.0.1
// with any port, for development convenience.
func IsLocalOrigin(origin string) bool {
	stripped := origin
	if idx := strings.Index(origin, "://"); idx != -1 {
		stripped = origin[idx+3:]
	}

	host := stripped
	if idx := strings.LastIndex(stripped, ":"); idx != -1 {
		host = stripped[:idx]
	}

	return host == "localhost" || host == "127.0.0.1" || host == "[::1]"
}

// isDefaultProductionOrigin checks against the production origins derived
// from JC_SITE_URL when JC_CORS_ORIGINS is not set.
func isDefaultProductionOrigin(origin, siteURL string) bool {
	if siteURL == "" {
		return false
	}

	domain := strings.TrimPrefix(strings.TrimPrefix(siteURL, "https://"), "http://")
	if idx := strings.Index(domain, "/"); idx != -1 {
		domain = domain[:idx]
	}

	for _, sub := range []string{"", "www.", "admin.", "control.", "docs.", "learn.", "api."} {
		if origin == "https://"+sub+domain {
			return true
		}
	}

	return false
}
