// This file provides SSRF hardening for the intelligence fetchers.
//
// The fetchers (api, rss, static) previously used a bare http.Client with only
// a Timeout set. That left three gaps:
//
//  1. First-hop SSRF: the entry URL itself (or a content-discovered URL —
//     a sitemap <loc>, a robots.txt Sitemap: declaration, a page <link>
//     feed) could point straight at an internal address; nothing validated
//     the target before the first request.
//  2. Redirect-based SSRF: a public URL could redirect (302) to an internal
//     address (e.g. http://127.0.0.1:8500/v1/...) letting an attacker probe
//     internal services through the crawler.
//  3. Unbounded response bodies: a hostile or misconfigured endpoint could
//     stream gigabytes, exhausting memory (DoS).
//
// NewSafeClient addresses all three: the transport's DialContext resolves
// the host and rejects private/internal IP literals AND DNS answers before
// any connection is opened (fail-closed on lookup failure), CheckRedirect
// keeps the redirect-time guard as a second layer, and MaxBodyReader caps
// the response body and surfaces an error when the cap is exceeded.
//
// Ref: BUG-14, intel-channels Phase 1 final review C1

package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
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
// as dial and redirect targets. The set mirrors
// internal/linkpreview/ssrf.go's privateIPRanges (CGNAT, ULA, multicast and
// the documentation/test ranges included) — the two packages cannot share
// the table because linkpreview imports internal/auth, which would close an
// import cycle through internal/worker → fetcher. Keep both in sync.
// mustParseCIDR is used so a bad literal fails at init time rather than
// silently disabling a check.
var privateNets = []net.IPNet{
	mustParseCIDR("127.0.0.0/8"),        // loopback
	mustParseCIDR("10.0.0.0/8"),         // private class A
	mustParseCIDR("172.16.0.0/12"),      // private class B
	mustParseCIDR("192.168.0.0/16"),     // private class C
	mustParseCIDR("169.254.0.0/16"),     // link-local
	mustParseCIDR("0.0.0.0/8"),          // current network
	mustParseCIDR("100.64.0.0/10"),      // shared address space (CGNAT)
	mustParseCIDR("192.0.0.0/24"),       // IETF protocol assignments
	mustParseCIDR("192.0.2.0/24"),       // TEST-NET-1
	mustParseCIDR("198.51.100.0/24"),    // TEST-NET-2
	mustParseCIDR("203.0.113.0/24"),     // TEST-NET-3
	mustParseCIDR("224.0.0.0/4"),        // IPv4 multicast
	mustParseCIDR("240.0.0.0/4"),        // reserved
	mustParseCIDR("255.255.255.255/32"), // broadcast
	mustParseCIDR("::1/128"),            // IPv6 loopback
	mustParseCIDR("fc00::/7"),           // IPv6 unique local
	mustParseCIDR("fe80::/10"),          // IPv6 link-local
	mustParseCIDR("ff00::/8"),           // IPv6 multicast
}

func mustParseCIDR(s string) net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("fetcher: invalid private CIDR %q: %v", s, err))
	}

	return *n
}

// safeTransport wraps an *http.Transport cloned from http.DefaultTransport
// (same pooling/dial defaults). With the SSRF guard on, the clone's
// DialContext is replaced by guardedDialContext so every outbound connection
// — first hop, redirect, or content-discovered URL — is validated against
// the private range set before a socket is opened. A transport cannot
// intercept redirects itself, so the client-level CheckRedirect stays as a
// second layer.
type safeTransport struct {
	base http.RoundTripper
}

// newSafeTransport returns a safeTransport. When allowPrivate is false the
// underlying transport's DialContext is wrapped with the private-IP dial
// guard; when true (test-only opt-out) the plain cloned transport is used
// so httptest servers bound to 127.0.0.1 stay reachable.
func newSafeTransport(allowPrivate bool) *safeTransport {
	t := http.DefaultTransport.(*http.Transport).Clone() //nolint:forcetypeassert // stdlib guarantees the concrete type
	if !allowPrivate {
		base := t.DialContext
		if base == nil {
			d := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
			base = d.DialContext
		}

		t.DialContext = guardedDialContext(base)
	}

	return &safeTransport{base: t}
}

// RoundTrip delegates to the underlying transport.
func (t *safeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.base.RoundTrip(req)
}

// ipLookup resolves a hostname to its addresses. It is a package-level var so
// tests can stub it — the DNS-answer rejection path must be verifiable
// without real DNS. Production uses the default resolver.
var ipLookup = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// guardedDialContext wraps a dial function with the SSRF private-IP guard:
//
//   - IP-literal hosts are checked directly (no DNS);
//   - named hosts are resolved and EVERY answer is checked — a single
//     private address rejects the dial (a public A record must not shield a
//     private AAAA, and vice versa);
//   - resolution failures and empty answer sets fail closed;
//   - on success the connection is opened to one of the VALIDATED addresses
//     (family-matched to the requested network), not re-resolved by the
//     underlying dialer — this closes the DNS-rebinding TOCTOU window where
//     a hostile domain could answer public IPs for the check and private IPs
//     for the real dial.
func guardedDialContext(base func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("fetcher: dial %q: %w", addr, err)
		}

		// IP literal (e.g. http://169.254.169.254/ or http://[::1]/): check
		// immediately, no DNS lookup needed. SplitHostPort already strips
		// the brackets from IPv6 literals.
		if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
			if isPrivateIP(ip) {
				return nil, fmt.Errorf("fetcher: dial to private IP %s rejected (SSRF)", ip)
			}

			return base(ctx, network, addr)
		}

		ips, err := ipLookup(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("fetcher: dial host %q lookup failed (SSRF fail-closed): %w", host, err)
		}

		if len(ips) == 0 {
			return nil, fmt.Errorf("fetcher: dial host %q resolved to no addresses (SSRF fail-closed)", host)
		}

		var chosen net.IP

		for _, a := range ips {
			if isPrivateIP(a.IP) {
				return nil, fmt.Errorf("fetcher: host %q resolves to private IP %s (SSRF)", host, a.IP)
			}

			// Pick the first validated address matching the requested
			// address family ("tcp4"/"tcp6"); plain "tcp" takes the first.
			if chosen == nil {
				switch network {
				case "tcp4":
					if a.IP.To4() != nil {
						chosen = a.IP
					}
				case "tcp6":
					if a.IP.To4() == nil {
						chosen = a.IP
					}
				default:
					chosen = a.IP
				}
			}
		}

		if chosen == nil {
			return nil, fmt.Errorf("fetcher: host %q has no %s-routable address", host, network)
		}

		return base(ctx, network, net.JoinHostPort(chosen.String(), port))
	}
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
//   - rejects any redirect whose resolved host is a private/internal IP —
//     skipped entirely when allowPrivate is set (test opt-out).
//
// Host resolution: if the URL host is already an IP literal it is checked
// directly (no DNS). Otherwise the host is resolved via net.LookupIP and every
// returned address is checked. A resolution failure fails closed (the redirect
// is rejected) to avoid silently following a redirect to an unresolvable
// internal name.
func checkRedirectURL(req *http.Request, via []*http.Request, allowPrivate bool) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("fetcher: stopped after %d redirects", maxRedirects)
	}

	if allowPrivate {
		return nil
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

// SafeClientOpts tunes NewSafeClientOpts. The zero value is the production
// configuration — every guard on.
type SafeClientOpts struct {
	// AllowPrivate disables the private-IP guards (dial-time and redirect).
	// It exists ONLY for tests: httptest servers bind to 127.0.0.1, which the
	// guard would otherwise reject. Never set it in production wiring —
	// server.go and the worker use NewSafeClient (guard on).
	AllowPrivate bool
}

// NewSafeClient returns an *http.Client hardened against first-hop and
// redirect-based SSRF plus unbounded response bodies:
//   - Transport: a safeTransport whose DialContext resolves every outbound
//     host and rejects private/internal IP literals and DNS answers before
//     connecting (and dials a validated address, so a hostile name cannot
//     re-resolve to a private IP between check and connect).
//   - Timeout: the supplied per-request timeout.
//   - CheckRedirect: checkRedirectURL, which blocks redirects to private IP
//     ranges and caps the chain at maxRedirects — a second layer behind the
//     dial guard.
//
// Callers still need to cap the response body themselves via MaxBodyReader
// (or NewMaxBodyReader) when reading resp.Body, since the transport cannot
// enforce a body cap before the application starts reading.
func NewSafeClient(timeout time.Duration, maxBody int64) *http.Client {
	return NewSafeClientOpts(timeout, maxBody, SafeClientOpts{})
}

// NewSafeClientOpts is NewSafeClient with explicit options. opts.AllowPrivate
// disables the private-IP guards — the documented test-only escape hatch for
// httptest targets on loopback.
func NewSafeClientOpts(timeout time.Duration, _ int64, opts SafeClientOpts) *http.Client {
	return &http.Client{
		Transport: newSafeTransport(opts.AllowPrivate),
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return checkRedirectURL(req, via, opts.AllowPrivate)
		},
	}
}
