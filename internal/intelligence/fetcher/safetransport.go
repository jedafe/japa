// safetransport.go provides SSRF hardening for the intelligence fetchers.
//
// The fetchers (api, rss, static) previously used a bare http.Client with only
// a Timeout set. That left two gaps:
//
//  1. Redirect-based SSRF: a public URL could redirect (302) to an internal
//     address (e.g. http://127.0.0.1:8500/v1/...) letting an attacker probe
//     internal services through the crawler.
//  2. Unbounded response bodies: a hostile or misconfigured endpoint could
//     stream gigabytes, exhausting memory (DoS).
//
// This file addresses both via NewSafeClient, which wires a CheckRedirect that
// rejects redirects to private/internal IP ranges, plus MaxBodyReader which
// caps the response body and surfaces an error when the cap is exceeded.
//
// Ref: BUG-14
package fetcher

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const (
	// defaultMaxBodySize is the maximum number of bytes a fetcher will read
	// from a single response body. 10MB is generous for HTML/RSS/JSON grant
	// sources while preventing memory exhaustion from hostile endpoints.
	defaultMaxBodySize int64 = 10 * 1024 * 1024 // 10MB

	// maxRedirects caps the number of HTTP redirects the client will follow,
	// matching net/http's default policy. Each redirect is also validated
	// against private IP ranges by checkRedirectURL.
	maxRedirects = 10
)

// ErrBodyTooLarge is returned by MaxBodyReader when the underlying body
// exceeds the configured limit. Callers should treat this as a fetch
// failure (the body is incomplete and must not be hashed/persisted).
var ErrBodyTooLarge = errors.New("fetcher: response body exceeds max size")

// privateNets are the IP ranges considered "internal" and therefore blocked
// as redirect targets. They cover IPv4 loopback/private/link-local/current-
// network plus IPv6 loopback and link-local. mustParseCIDR is used so a bad
// literal fails at init time rather than silently disabling a check.
var privateNets = []net.IPNet{
	mustParseCIDR("127.0.0.0/8"),    // loopback
	mustParseCIDR("10.0.0.0/8"),     // private class A
	mustParseCIDR("172.16.0.0/12"),  // private class B
	mustParseCIDR("192.168.0.0/16"), // private class C
	mustParseCIDR("169.254.0.0/16"), // link-local
	mustParseCIDR("0.0.0.0/8"),      // current network
	mustParseCIDR("::1/128"),        // IPv6 loopback
	mustParseCIDR("fe80::/10"),      // IPv6 link-local
}

func mustParseCIDR(s string) net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("fetcher: invalid private CIDR %q: %v", s, err))
	}
	return *n
}

// safeTransport wraps http.DefaultTransport so that all safe clients share the
// default connection pooling/dial settings. The actual SSRF mitigation lives
// in the client-level CheckRedirect (set by NewSafeClient); a transport cannot
// intercept redirects itself.
type safeTransport struct {
	base http.RoundTripper
}

// newSafeTransport returns a safeTransport backed by http.DefaultTransport.
func newSafeTransport() *safeTransport {
	return &safeTransport{base: http.DefaultTransport}
}

// RoundTrip delegates to the underlying transport.
func (t *safeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req)
}

// MaxBodyReader wraps an io.Reader and limits the total bytes read to limit.
// Unlike a plain io.LimitReader, it returns ErrBodyTooLarge when the source
// contains more than limit bytes, so callers can distinguish a complete body
// from a truncated one (silent truncation would produce a wrong content hash).
//
// It uses io.LimitReader internally, reading up to limit+1 bytes: if the
// (limit+1)th byte is ever produced, the body is too large.
type MaxBodyReader struct {
	r     io.Reader
	limit int64
	read  int64
}

// NewMaxBodyReader returns a MaxBodyReader that caps reads from r at limit
// bytes. A non-positive limit allows no body bytes.
func NewMaxBodyReader(r io.Reader, limit int64) *MaxBodyReader {
	if limit < 0 {
		limit = 0
	}
	return &MaxBodyReader{r: io.LimitReader(r, limit+1), limit: limit}
}

// Read implements io.Reader. It returns ErrBodyTooLarge once more than limit
// bytes have been read from the underlying source.
func (m *MaxBodyReader) Read(p []byte) (int, error) {
	n, err := m.r.Read(p)
	m.read += int64(n)
	if m.read > m.limit {
		return n, ErrBodyTooLarge
	}
	return n, err
}

// isPrivateIP reports whether ip falls in any private/internal range. It also
// rejects the unspecified (0.0.0.0/::) and loopback/link-local addresses via
// the standard net.IP helpers, and normalizes IPv4-mapped IPv6 addresses so
// they are caught by the IPv4 CIDRs.
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return true
	}
	// Normalize IPv4-mapped IPv6 (::ffff:a.b.c.d) to 4-byte form so the IPv4
	// CIDR ranges below match it.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// checkRedirectURL is the CheckRedirect callback used by NewSafeClient. It:
//   - caps the redirect chain at maxRedirects (matching net/http's default);
//   - rejects any redirect whose resolved host is a private/internal IP.
//
// Host resolution: if the URL host is already an IP literal it is checked
// directly (no DNS). Otherwise the host is resolved via net.LookupIP and every
// returned address is checked. A resolution failure fails closed (the redirect
// is rejected) to avoid silently following a redirect to an unresolvable
// internal name.
func checkRedirectURL(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("fetcher: stopped after %d redirects", maxRedirects)
	}

	host := req.URL.Hostname()
	if host == "" {
		// Relative/no-host redirect — nothing to validate; let the client
		// resolve it against the base URL.
		return nil
	}

	// IP literal: check immediately, no DNS lookup needed.
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("fetcher: redirect to private IP %s rejected (SSRF)", ip)
		}
		return nil
	}

	// Hostname: resolve and check every address (a single private address
	// among the A/AAAA records is enough to reject).
	addrs, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("fetcher: redirect host %q lookup failed (SSRF fail-closed): %w", host, err)
	}
	for _, a := range addrs {
		if isPrivateIP(a) {
			return fmt.Errorf("fetcher: redirect to private IP %s (host %q) rejected (SSRF)", a, host)
		}
	}
	return nil
}

// NewSafeClient returns an *http.Client hardened against redirect-based SSRF
// and unbounded response bodies:
//   - Transport: a safeTransport wrapping http.DefaultTransport (shared pool).
//   - Timeout: the supplied per-request timeout.
//   - CheckRedirect: checkRedirectURL, which blocks redirects to private IP
//     ranges and caps the chain at maxRedirects.
//
// Callers still need to cap the response body themselves via MaxBodyReader
// (or NewMaxBodyReader) when reading resp.Body, since the transport cannot
// enforce a body cap before the application starts reading.
func NewSafeClient(timeout time.Duration, maxBodySize int64) *http.Client {
	return &http.Client{
		Transport:     newSafeTransport(),
		Timeout:       timeout,
		CheckRedirect: checkRedirectURL,
	}
}
