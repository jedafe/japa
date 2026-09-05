// Package clientip resolves the originating IP address of an HTTP request.
//
// X-Forwarded-For and X-Real-IP are client-controlled headers: anyone can set
// them. They may only be believed when the immediate peer (RemoteAddr) is a
// reverse proxy we operate. Everything that makes a security decision on an IP
// address — brute-force lockouts, rate limits, IP blocklists, API key
// allowlists — must resolve the address through this package, otherwise the
// control is bypassed by sending a header.
//
// The trusted proxy set is configured with JC_TRUSTED_PROXIES (comma-separated
// CIDRs or bare IPs). It defaults to loopback and the RFC1918 / RFC4193 ranges,
// which covers the usual "ingress in front of the app inside a private
// network" deployment.
package clientip

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

// defaultTrustedCIDRs are the ranges a reverse proxy normally lives in.
var defaultTrustedCIDRs = []string{
	"127.0.0.0/8",
	"::1/128",
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"fc00::/7",
}

// trustedPtr holds the parsed trusted-proxy CIDRs. It is populated lazily on
// the first call to trustedProxies() and stored atomically so that reads are
// lock-free. Tests can call resetForTesting to force re-initialization with a
// different JC_TRUSTED_PROXIES value.
var trustedPtr atomic.Pointer[[]*net.IPNet]

// trustedProxies parses JC_TRUSTED_PROXIES once and caches the result.
func trustedProxies() []*net.IPNet {
	if p := trustedPtr.Load(); p != nil {
		return *p
	}

	nets := parseTrustedProxyList()
	trustedPtr.Store(&nets)

	return nets
}

// parseTrustedProxyList reads JC_TRUSTED_PROXIES from the environment and
// returns the parsed CIDR list. When the env var is empty the default CIDRs
// (loopback + RFC1918 + RFC4193) are used. Invalid entries are skipped with a
// warning log so that a misconfigured proxy list is visible to operators — a
// silently dropped CIDR would leave a real proxy untrusted, causing forwarding
// headers to be ignored and rate limits / IP blocks to key on the proxy address
// instead of the real client.
func parseTrustedProxyList() []*net.IPNet {
	entries := defaultTrustedCIDRs
	source := "defaults"

	if env := strings.TrimSpace(os.Getenv("JC_TRUSTED_PROXIES")); env != "" {
		entries = strings.Split(env, ",")
		source = "JC_TRUSTED_PROXIES"
	}

	var nets []*net.IPNet

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if !strings.Contains(entry, "/") {
			if ip := net.ParseIP(entry); ip != nil {
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}

				nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			} else {
				slog.Warn("clientip: skipping invalid trusted proxy entry (not a valid IP)",
					"entry", entry, "source", source)
			}

			continue
		}

		if _, network, err := net.ParseCIDR(entry); err == nil {
			nets = append(nets, network)
		} else {
			slog.Warn("clientip: skipping invalid trusted proxy CIDR",
				"entry", entry, "source", source, "error", err)
		}
	}

	slog.Info("clientip: trusted proxy set loaded",
		"source", source, "cidr_count", len(nets))

	return nets
}

// resetForTesting clears the cached trusted-proxy set so that a subsequent
// call to trustedProxies() re-reads JC_TRUSTED_PROXIES. It is intended for
// test use only and must not be called while HTTP requests are being served.
func resetForTesting() {
	trustedPtr.Store(nil)
}

// IsTrustedProxy reports whether an IP belongs to the configured proxy set.
func IsTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}

	for _, network := range trustedProxies() {
		if network.Contains(ip) {
			return true
		}
	}

	return false
}

// PeerIP returns the immediate TCP peer address of the request in canonical
// form, without consulting any forwarding headers. This is the address used
// to decide whether X-Forwarded-* headers may be trusted.
//
// Callers that need the actual client address should use FromRequest; PeerIP
// is exposed for consumers that must gate peer-derived behaviour on the
// trusted-proxy model themselves — for example, refusing to build a
// password-reset link from r.Host when the peer is not a trusted proxy,
// because r.Host is the client-controllable Host header and using it from
// an untrusted peer reopens host-header poisoning (CWE-601).
func PeerIP(r *http.Request) string {
	return peerIP(r)
}

// IsTrustedPeer reports whether the immediate TCP peer of the request belongs
// to the configured trusted-proxy set. It is a convenience wrapper over
// PeerIP and IsTrustedProxy that handles the empty / unparseable RemoteAddr
// edge cases (returning false in both), so consumers can gate
// peer-derived behaviour without re-implementing the peer extraction.
func IsTrustedPeer(r *http.Request) bool {
	peer := peerIP(r)
	if peer == "" {
		return false
	}

	return IsTrustedProxy(net.ParseIP(peer))
}

// FromRequest returns the client IP for the request in canonical form.
// Forwarding headers are honoured only when the connecting peer is a trusted
// proxy; otherwise the peer address itself is returned. The returned string
// is always the canonical representation produced by net.IP.String() so that
// downstream consumers (rate-limit keys, hashes, cache keys, geoip lookups)
// see a consistent value for the same address regardless of how the header
// encoded it.
func FromRequest(r *http.Request) string {
	peer := peerIP(r)
	if peer == "" || !IsTrustedProxy(net.ParseIP(peer)) {
		return peer
	}

	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// The right-most entry is the one appended by our own proxy; walk
		// leftwards past any further proxies we trust and take the first
		// address the chain did not vouch for.
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])

			ip := net.ParseIP(candidate)
			if ip == nil {
				continue
			}

			if !IsTrustedProxy(ip) {
				return ip.String()
			}
		}
	}

	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		if ip := net.ParseIP(xri); ip != nil {
			return ip.String()
		}
	}

	return peer
}

// peerIP returns the address of the immediate TCP peer, without its port, in
// canonical form. If the host portion is a valid IP it is normalized via
// net.IP.String(); otherwise the raw string is returned as-is.
func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}

	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}

	return host
}

// ForwardedBaseURL returns the public-facing base URL (scheme://host) for the
// request, honouring X-Forwarded-Host and X-Forwarded-Proto only when the
// immediate peer is a trusted proxy. When the peer is untrusted (a direct
// client connection), or when no forwarding headers are present, the fallback
// is returned unchanged.
//
// This applies the same trusted-proxy gate as FromRequest: X-Forwarded-Host
// and X-Forwarded-Proto are client-controlled headers that anyone can set.
// Honoring them unconditionally enables host-header injection — an attacker
// sends X-Forwarded-Host: evil.com and RSS feed links, password-reset links,
// or other generated URLs point to a domain they control. Every consumer that
// builds a public URL from forwarding headers must route through this helper
// (or ForwardedScheme/ForwardedHost) so the trusted-proxy model is enforced
// consistently.
//
// The scheme defaults to "https" when X-Forwarded-Proto is absent and the
// request was received over TLS; otherwise "http". This matches the behaviour
// of the previous inline implementations in blog/rss/auth handlers.
func ForwardedBaseURL(r *http.Request, fallback string) string {
	host := ForwardedHost(r)
	if host == "" {
		return fallback
	}

	return ForwardedScheme(r) + "://" + host
}

// ForwardedHost returns the X-Forwarded-Host value when the immediate peer is
// a trusted proxy; otherwise it returns an empty string so callers fall back
// to a configured or peer-derived value.
//
// The header value is sanitized before it is returned: only a syntactically
// valid host (with optional port) is accepted. Values containing control
// characters, whitespace, path/query separators, or backslashes are rejected
// and the function returns "". This is defense in depth on top of the
// trusted-proxy gate: the header is client-influenced, and these values are
// concatenated into public URLs (RSS link elements, password-reset email
// links). A CRLF-bearing host can split HTTP headers or RSS XML lines; a
// path-bearing host can inject an attacker-controlled path into a base URL;
// an embedded-space host breaks URL parsing. Rejecting them here keeps every
// consumer safe without each one re-validating.
func ForwardedHost(r *http.Request) string {
	peer := peerIP(r)
	if peer == "" || !IsTrustedProxy(net.ParseIP(peer)) {
		return ""
	}

	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if !isValidForwardedHost(host) {
		return ""
	}

	return host
}

// isValidForwardedHost reports whether s is a syntactically valid host
// (optionally with a port) suitable for concatenation into a URL. It rejects
// the empty string, control characters (incl. CR/LF/tab), any whitespace,
// and the path/query/fragment separators "/", "?", "#", and backslash. A
// bare bracketed IPv6 address or a bracketed IPv6 with a port is accepted.
func isValidForwardedHost(s string) bool {
	if s == "" {
		return false
	}

	for i := range len(s) {
		c := s[i]
		// Reject control chars (incl. CR/LF/tab) and any whitespace.
		if c < 0x21 || c == 0x7f {
			return false
		}
		// Reject path/query/fragment separators and backslash. These turn
		// a host into a host+path and let an attacker steer the path
		// component of a generated URL.
		switch c {
		case '/', '?', '#', '\\':
			return false
		}
	}

	return true
}

// ForwardedScheme returns the transport scheme for the request, honouring
// X-Forwarded-Proto only when the immediate peer is a trusted proxy. When
// the header is absent, the peer is untrusted, or the header value is not
// exactly "http" or "https" (case-insensitive), it falls back to "https"
// when the request was received over TLS, otherwise "http".
//
// The value is validated, not just gated: X-Forwarded-Proto is
// client-influenced and the returned scheme is concatenated into public URLs
// (RSS links, password-reset email links). Returning an arbitrary string
// such as "javascript:alert(1)//" would produce a javascript:// link in an
// email; a CRLF-bearing value could split HTTP headers or RSS XML lines.
// Only "http" and "https" are ever returned from the header.
func ForwardedScheme(r *http.Request) string {
	peer := peerIP(r)
	if peer != "" && IsTrustedProxy(net.ParseIP(peer)) {
		switch strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))) {
		case "http":
			return "http"
		case "https":
			return "https"
		}
	}

	if r.TLS != nil {
		return "https"
	}

	return "http"
}
