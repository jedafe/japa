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
// (loopback + RFC1918 + RFC4193) are used.
func parseTrustedProxyList() []*net.IPNet {
	entries := defaultTrustedCIDRs
	if env := strings.TrimSpace(os.Getenv("JC_TRUSTED_PROXIES")); env != "" {
		entries = strings.Split(env, ",")
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
			}

			continue
		}

		if _, network, err := net.ParseCIDR(entry); err == nil {
			nets = append(nets, network)
		}
	}

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
