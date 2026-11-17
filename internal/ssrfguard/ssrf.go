// Package ssrfguard provides shared SSRF protections for outbound HTTP
// fetches: private/reserved IP-range checks, URL validation, redirect
// revalidation, and an http.Client that blocks private IPs at dial time
// (closing the DNS-rebinding TOCTOU window between validation and
// connection). Leaf package — safe for any internal package to import
// (worker, visaroutes, media, email, linkpreview).
package ssrfguard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// privateIPRanges are the CIDR blocks that are blocked by SSRF protection.
// These cover all private/reserved IP ranges that should never be fetched.
var privateIPRanges = []string{
	"10.0.0.0/8",         // Class A private
	"172.16.0.0/12",      // Class B private
	"192.168.0.0/16",     // Class C private
	"127.0.0.0/8",        // Loopback
	"169.254.0.0/16",     // Link-local
	"0.0.0.0/8",          // Current network
	"100.64.0.0/10",      // Shared address space (CGNAT)
	"192.0.0.0/24",       // IETF Protocol Assignments
	"192.0.2.0/24",       // TEST-NET-1
	"198.51.100.0/24",    // TEST-NET-2
	"203.0.113.0/24",     // TEST-NET-3
	"224.0.0.0/4",        // Multicast
	"240.0.0.0/4",        // Reserved
	"255.255.255.255/32", // Broadcast
	"::1/128",            // IPv6 loopback
	"fc00::/7",           // IPv6 unique local
	"fe80::/10",          // IPv6 link-local
}

// MaxRedirectDepth is the maximum number of redirects the safe client
// follows before failing the request.
const MaxRedirectDepth = 5

// ErrRedirectToPrivateIP is returned when a redirect target resolves to
// a private/reserved IP range.
var ErrRedirectToPrivateIP = errors.New("redirect target resolves to private IP")

// IsPrivateIP returns true if the given IP address is in a private/reserved range.
// IPv4-mapped IPv6 forms (::ffff:127.0.0.1) are normalized to IPv4 first:
// without To4, a 16-byte mapped address tested against a 4-byte CIDR can
// miss and pass validation while the dialer treats it as 127.0.0.1.
func IsPrivateIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	for _, cidr := range privateIPRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}

		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// ValidateURL checks whether a URL is safe to fetch (not SSRF).
// It resolves the hostname and checks that the IP is not in a private range.
func ValidateURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http and https schemes
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid scheme: %s (only http and https allowed)", u.Scheme)
	}

	// Resolve hostname to IP
	host := u.Hostname()
	if host == "" {
		return errors.New("empty hostname")
	}

	// Check if hostname is an IP address
	if ip := net.ParseIP(host); ip != nil {
		if IsPrivateIP(ip) {
			return fmt.Errorf("private IP address not allowed: %s", ip)
		}

		return nil
	}

	// Resolve hostname via DNS
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS lookup failed: %w", err)
	}

	// Check all resolved IPs
	for _, ip := range ips {
		if IsPrivateIP(ip) {
			return fmt.Errorf("hostname resolves to private IP: %s -> %s", host, ip)
		}
	}

	return nil
}

// ValidateRedirectURL checks whether a redirect target URL is safe.
// It validates the scheme and resolves the hostname to ensure no redirect
// leads to a private/reserved IP range. This prevents redirect-based SSRF
// where the initial URL passes validation but redirects to an internal host.
func ValidateRedirectURL(urlStr string) error {
	u, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid redirect URL: %w", err)
	}

	// Only allow http and https schemes on redirects.
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid redirect scheme: %s (only http and https allowed)", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return errors.New("redirect target has empty hostname")
	}

	// If hostname is already an IP, check it directly.
	if ip := net.ParseIP(host); ip != nil {
		if IsPrivateIP(ip) {
			return fmt.Errorf("redirect target resolves to private IP: %s", ip)
		}

		return nil
	}

	// Resolve hostname via DNS and check all IPs.
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("DNS lookup failed for redirect target %s: %w", host, err)
	}

	for _, ip := range ips {
		if IsPrivateIP(ip) {
			return fmt.Errorf("redirect target resolves to private IP: %s -> %s", host, ip)
		}
	}

	return nil
}

// ssrfSafeTransport wraps an inner RoundTripper. The inner transport uses a
// custom Dialer with a Control function that blocks connections to private IP
// ranges at dial time (DNS rebinding protection). CheckRedirect
// validates each redirect hop's URL before following it.
type ssrfSafeTransport struct {
	inner http.RoundTripper
}

func (t *ssrfSafeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.inner.RoundTrip(req)
}

// NewSafeHTTPClient returns an *http.Client with the platform egress guard:
// dial-time private-IP blocking (DNS-rebinding protection) and per-hop
// redirect validation with a redirect depth cap. Shared by every
// user/data-triggered fetch path — link previews, remote media, and the
// visa-route/credential gov-URL link checks — so no caller rolls its own
// unprotected client. Callers set per-request timeouts via context.
func NewSafeHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("unparseable IP at connection time: %s", host)
			}
			if IsPrivateIP(ip) {
				return fmt.Errorf("connection to private IP blocked at dial time: %s", ip)
			}
			return nil
		},
	}

	transport := &ssrfSafeTransport{
		inner: &http.Transport{
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= MaxRedirectDepth {
				return fmt.Errorf("stopped after %d redirects", MaxRedirectDepth)
			}
			if err := ValidateRedirectURL(req.URL.String()); err != nil {
				return ErrRedirectToPrivateIP
			}
			return nil
		},
	}
}
