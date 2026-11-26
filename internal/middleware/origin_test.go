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

// TestIsOriginAllowedDevEnvHonorsAllowlist verifies that a development server
// deployed on a real domain (e.g. dev.japa.to) accepts its configured origins
// in addition to localhost — the dev-env early return used to ignore the
// allowlist entirely, which broke WebSocket CheckOrigin (and would break CORS)
// on every deployed dev host.
func TestIsOriginAllowedDevEnvHonorsAllowlist(t *testing.T) {
	t.Setenv("JC_ENV", "development")
	t.Setenv("JC_CORS_ORIGINS", "https://dev.japa.to")
	t.Setenv("JC_SITE_URL", "")

	if !IsOriginAllowed("https://dev.japa.to") {
		t.Error("configured deployed origin rejected in dev env")
	}
	if !IsOriginAllowed("http://localhost:3000") {
		t.Error("localhost origin rejected in dev env")
	}
	if IsOriginAllowed("https://evil.example.com") {
		t.Error("unconfigured origin accepted in dev env")
	}
	if IsOriginAllowed("*") {
		t.Error("wildcard accepted in dev env")
	}
}

// TestIsOriginAllowedProductionGates verifies production still rejects
// non-allowlisted origins and localhost.
func TestIsOriginAllowedProductionGates(t *testing.T) {
	t.Setenv("JC_ENV", "production")
	t.Setenv("JC_CORS_ORIGINS", "")
	t.Setenv("JC_SITE_URL", "https://japa.to")

	if !IsOriginAllowed("https://japa.to") {
		t.Error("site origin rejected in production")
	}
	if !IsOriginAllowed("https://admin.japa.to") {
		t.Error("subdomain origin rejected in production")
	}
	if IsOriginAllowed("http://localhost:3000") {
		t.Error("localhost accepted in production")
	}
	if IsOriginAllowed("https://evil.example.com") {
		t.Error("unconfigured origin accepted in production")
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
