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

var defaultCORSHeaders = []string{
	"Content-Type",
	"Authorization",
	"Idempotency-Key",
	"X-Request-ID",
	"X-CSRF-Token",
	"X-API-Key",
	"X-Device-Id",
}

// AllowedCORSHeaders returns the Access-Control-Allow-Headers value used by
// the CORS middleware. The default list covers the standard headers used by
// the web and mobile clients plus the desktop studio's X-API-Key and
// X-Device-Id headers. Additional headers can be added with JC_CORS_HEADERS.
func AllowedCORSHeaders() string {
	if env := os.Getenv("JC_CORS_HEADERS"); env != "" {
		extras := strings.Split(env, ",")

		headers := append([]string{}, defaultCORSHeaders...)

		for _, h := range extras {
			h = strings.TrimSpace(h)
			if h == "" {
				continue
			}

			duplicate := false

			for _, existing := range headers {
				if strings.EqualFold(existing, h) {
					duplicate = true
					break
				}
			}

			if !duplicate {
				headers = append(headers, h)
			}
		}

		return strings.Join(headers, ", ")
	}

	return strings.Join(defaultCORSHeaders, ", ")
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

	// Subdomains for the ADR-086 multi-surface client architecture (§78).
	// V1 app surfaces + V1 identity tools + V2 functional tools (§§90–101).
	// These are the default subdomain prefixes; custom domains are handled
	// via the JC_CORS_ORIGINS env var or cors.allowed_origins system setting.
	for _, sub := range []string{
		// Core app surfaces
		"", "www.", "admin.", "control.", "docs.", "academy.", "api.",
		// V1 identity tools (§§70–74)
		"tools.", "remittance.", "scholarships.", "cost-of-living.",
		"visa-checklist.", "readiness.", "japabot.",
		// V2 functional tools (§§90–97, §99–§101)
		"visa-routes.", "appointment-alerts.", "arrival-checklist.",
		"processing-times.", "credentials.", "cv-formatter.",
		"housing-guide.", "attestation.", "language-tests.",
		"tax-calculator.", "community-locator.",
	} {
		if origin == "https://"+sub+domain {
			return true
		}
	}

	return false
}
