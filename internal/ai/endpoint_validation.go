// Package ai — endpoint_validation.go provides URL validation for all LLM
// provider HTTP calls. gosec's G704 (SSRF via taint analysis) flags every
// http.NewRequestWithContext call that uses a URL derived from a struct
// field, because it cannot prove the field wasn't set from user input.
//
// The provider base URLs come from admin-configured system_settings (Rule 3),
// not from user input, so the SSRF risk is low. However, defense-in-depth
// requires that we validate the URL at the point of use rather than relying
// on the assumption that admins won't misconfigure it. If a future feature
// allows user-configured custom LLM endpoints, this validation prevents an
// attacker from redirecting requests to internal services.
//
// validateEndpoint enforces:
//   - URL must parse successfully
//   - Scheme must be https (or http only when explicitly allowed for local
//     development proxies — see validateEndpointAllowHTTP)
//   - Host must be non-empty
//   - Host must not be a bare IP address in production (prevents SSRF to
//     internal services like 169.254.169.254, localhost, etc.)
//
// Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Security)
package ai

import (
	"context"
	"errors"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// errInvalidEndpoint is returned when an endpoint URL fails validation.
// The error message is deliberately generic to avoid leaking whether the
// URL was an internal address, a bare IP, or malformed.
var errInvalidEndpoint = errors.New("ai: invalid or disallowed endpoint URL")

// validateEndpoint validates that an endpoint URL is safe to make HTTP
// requests to. It enforces HTTPS, non-empty host, and rejects bare IP
// addresses and internal/loopback hosts. Use this for all production LLM
// provider calls.
func validateEndpoint(rawURL string) error {
	return validateEndpointWithPolicy(rawURL, false)
}

// validateEndpointAllowHTTP validates an endpoint URL but permits HTTP
// scheme. Use this only for local development proxies where HTTPS is not
// available (e.g. a local Ollama instance at http://localhost:11434).
// Internal/loopback hosts are still allowed in this mode since the
// development proxy is expected to be local.
func validateEndpointAllowHTTP(rawURL string) error {
	return validateEndpointWithPolicy(rawURL, true)
}

// validateEndpointWithPolicy is the shared validation logic. When
// allowHTTP is true, HTTP scheme is permitted for loopback hosts only
// (development mode — local LLM proxies like Ollama at
// http://localhost:11434). When false, HTTPS is required and all
// internal hosts are rejected (production mode).
//
// SSRF protection applies to ALL schemes: non-loopback internal hosts
// (private ranges, link-local/cloud-metadata, unspecified) are rejected
// for both HTTP and HTTPS. Only loopback (127.0.0.0/8, ::1, localhost)
// is permitted with HTTP — this covers the local development proxy use
// case while blocking SSRF to cloud metadata (169.254.169.254) and
// internal services (10.x, 172.16.x, 192.168.x) via HTTP.
func validateEndpointWithPolicy(rawURL string, allowHTTP bool) error {
	if strings.TrimSpace(rawURL) == "" {
		return errInvalidEndpoint
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return errInvalidEndpoint
	}

	// Scheme enforcement.
	if allowHTTP {
		if u.Scheme != "https" && u.Scheme != "http" {
			return errInvalidEndpoint
		}
	} else {
		if u.Scheme != "https" {
			return errInvalidEndpoint
		}
	}

	// Host must be non-empty.
	host := u.Hostname()
	if host == "" {
		return errInvalidEndpoint
	}

	// SSRF protection: reject internal hosts for ALL schemes.
	// For HTTPS: all internal hosts (including loopback) are rejected.
	// For HTTP (allowHTTP only): non-loopback internal hosts are rejected;
	// loopback is allowed for local development proxies (Ollama).
	if isInternalHost(host) {
		if u.Scheme == "https" || !isLoopbackHost(host) {
			return errInvalidEndpoint
		}
	}

	return nil
}

// isInternalHost reports whether the host is a loopback, private, or
// link-local address — these should never be valid LLM provider endpoints
// in production. Also rejects bare "localhost" hostname.
//
// Alt-IP forms (decimal 2130706433, hex 0x7f.0.0.1, octal 0177.0.0.1)
// bypass net.ParseIP and would otherwise pass as "hostnames" — they are
// normalized first: anything that parses as an IP after stripping is
// checked as an IP. Hostnames that need DNS to resolve (evil-rebind.com)
// cannot be decided here; the redirect cap and per-request timeout bound
// that residual, and provider URLs are admin-supplied (not attacker input).
func isInternalHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}

	// Try to parse as IP address — if it's not an IP, it's a hostname
	// and we only check for localhost above.
	ip := net.ParseIP(host)
	if ip == nil {
		if normalized := normalizeNumericIP(host); normalized != nil {
			ip = normalized
		} else {
			return false
		}
	}

	// Reject loopback (127.0.0.0/8, ::1), private (10.x, 172.16-31.x,
	// 192.168.x, fc00::/7), and link-local (169.254.x, fe80::/10).
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}

	// Reserved ranges the stdlib predicates miss.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}

		if v4[0] == 0 || v4[0] >= 240 {
			return true
		}

		if (v4[0] == 192 && v4[1] == 0 && (v4[2] == 0 || v4[2] == 2)) ||
			(v4[0] == 198 && v4[1] == 51 && v4[2] == 100) ||
			(v4[0] == 203 && v4[1] == 0 && v4[2] == 113) ||
			(v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)) {
			return true
		}
	}

	return false
}

// normalizeNumericIP parses decimal/hex/octal IPv4 forms that net.ParseIP
// rejects but the dialer accepts: "2130706433" (127.0.0.1),
// "0x7f.0.0.1", "0177.0.0.1". Returns nil when the host is not a numeric
// IPv4 form.
func normalizeNumericIP(host string) net.IP {
	if host == "" || strings.Contains(host, ":") {
		return nil
	}

	parts := strings.Split(host, ".")
	if len(parts) != 1 && len(parts) != 4 {
		return nil
	}

	parsePart := func(s string, bits int) (uint64, bool) {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0, false
		}

		base := 10
		if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
			base = 16
			s = s[2:]
		} else if len(s) > 1 && strings.HasPrefix(s, "0") {
			base = 8
			s = s[1:]
		}

		if s == "" {
			return 0, false
		}

		n, err := strconv.ParseUint(s, base, bits)
		if err != nil {
			return 0, false
		}

		return n, true
	}

	if len(parts) == 1 {
		n, ok := parsePart(parts[0], 32)
		if !ok {
			return nil
		}

		return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}

	var b [4]byte

	for i, p := range parts {
		n, ok := parsePart(p, 8)
		if !ok {
			return nil
		}

		b[i] = byte(n)
	}

	return net.IPv4(b[0], b[1], b[2], b[3])
}

// isLoopbackHost reports whether the host is a loopback address
// (127.0.0.0/8, ::1) or the bare "localhost" hostname. This is used to
// allow HTTP to loopback for local development proxies (Ollama) while
// rejecting HTTP to other internal hosts (cloud metadata, private
// ranges) for SSRF defense.
func isLoopbackHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		ip = normalizeNumericIP(host)
	}

	if ip == nil {
		return false
	}

	return ip.IsLoopback()
}

// safeInt32 converts an int to int32 without risking integer overflow.
// If the value exceeds int32 range, it returns math.MaxInt32 (for positive
// values) or math.MinInt32 (for negative values). Use this instead of
// direct int32(n) conversions to satisfy gosec G115.
func safeInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}

	if n < math.MinInt32 {
		return math.MinInt32
	}

	return int32(n)
}

// safeInt64ToInt32 converts an int64 to int32 without risking integer
// overflow. If the value exceeds int32 range, it returns math.MaxInt32
// (for positive values) or math.MinInt32 (for negative values). Use this
// instead of direct int32(n) conversions to satisfy gosec G115.
func safeInt64ToInt32(n int64) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}

	if n < math.MinInt32 {
		return math.MinInt32
	}

	return int32(n)
}

// newValidatedHTTPRequest creates an *http.Request after validating the
// endpoint URL against the SSRF policy (validateEndpointAllowHTTP). This is
// the single point where gosec G704 (SSRF via taint analysis) is suppressed:
// the URL is validated immediately before the request is created, so the
// sink is provably guarded.
//
// Provider base URLs are validated at construction time in
// NewProviderFromConfig (admin-supplied URLs) or auto-selected from a
// hardcoded allowlist (Xiaomi). This call-site validation is defense-in-
// depth: it catches any path that bypasses the constructor (e.g. tests
// setting baseURL directly) and makes the guard visible at the sink.
//
// validateEndpointAllowHTTP permits HTTP only to loopback addresses
// (localhost, 127.0.0.0/8, ::1) for local development proxies (Ollama).
// Non-loopback internal hosts (cloud metadata 169.254.169.254, private
// ranges 10.x/172.16.x/192.168.x) are rejected for all schemes.
func newValidatedHTTPRequest(ctx context.Context, method, rawURL string, body io.Reader) (*http.Request, error) {
	if err := validateEndpointAllowHTTP(rawURL); err != nil {
		return nil, err
	}

	return http.NewRequestWithContext(ctx, method, rawURL, body) // #nosec G704 -- rawURL validated by validateEndpointAllowHTTP above
}
