package middleware

import (
	"strings"
	"testing"
)

// TestAllowedCORSHeadersDefaults pins the default Access-Control-Allow-Headers
// list. Every header a JapaConnect client may send must appear here or
// cross-origin requests (browser extension, admin SPA, docker-compose
// deployments) fail CORS preflight.
func TestAllowedCORSHeadersDefaults(t *testing.T) {
	t.Setenv("JC_CORS_HEADERS", "")

	allowed := AllowedCORSHeaders()

	for _, want := range []string{
		"Content-Type",
		"Authorization",
		"Idempotency-Key",
		"X-Request-ID",
		"X-CSRF-Token",
		"X-API-Key",
		"X-Device-Id",
		"X-JC-Surface",
	} {
		found := false

		for _, h := range strings.Split(allowed, ", ") {
			if strings.EqualFold(h, want) {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("AllowedCORSHeaders() missing %q; got %q", want, allowed)
		}
	}
}

// TestAllowedCORSHeadersEnvMerges verifies that JC_CORS_HEADERS appends
// extras to the default list rather than replacing it, and dedupes
// case-insensitively.
func TestAllowedCORSHeadersEnvMerges(t *testing.T) {
	t.Setenv("JC_CORS_HEADERS", "X-Custom-Thing, x-jc-surface, ,  ")

	allowed := AllowedCORSHeaders()
	headers := strings.Split(allowed, ", ")

	if !containsHeader(headers, "X-Custom-Thing") {
		t.Errorf("env header X-Custom-Thing not merged; got %q", allowed)
	}

	if !containsHeader(headers, "X-JC-Surface") {
		t.Errorf("default X-JC-Surface lost after env merge; got %q", allowed)
	}

	if !containsHeader(headers, "Authorization") {
		t.Errorf("default Authorization lost after env merge; got %q", allowed)
	}

	count := 0

	for _, h := range headers {
		if strings.EqualFold(h, "X-JC-Surface") {
			count++
		}
	}

	if count != 1 {
		t.Errorf("X-JC-Surface appears %d times, want 1; got %q", count, allowed)
	}
}

func containsHeader(headers []string, want string) bool {
	for _, h := range headers {
		if strings.EqualFold(h, want) {
			return true
		}
	}

	return false
}
