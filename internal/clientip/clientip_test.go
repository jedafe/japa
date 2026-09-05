package clientip

import (
	"context"
	"net"
	"net/http"
	"testing"
)

const testClientIP = "1.2.3.4"

func requestWith(remoteAddr string, headers map[string]string) *http.Request {
	r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr

	for k, v := range headers {
		r.Header.Set(k, v)
	}

	return r
}

func TestFromRequest_IgnoresSpoofedHeadersFromUntrustedPeer(t *testing.T) {
	t.Parallel()

	r := requestWith("203.0.113.10:4321", map[string]string{
		"X-Forwarded-For": testClientIP,
		"X-Real-IP":       "5.6.7.8",
	})

	if got := FromRequest(r); got != "203.0.113.10" {
		t.Fatalf("expected peer address, got %q", got)
	}
}

func TestFromRequest_TrustsProxyForwardedFor(t *testing.T) {
	t.Parallel()

	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-For": testClientIP + ", 10.0.0.5",
	})

	if got := FromRequest(r); got != testClientIP {
		t.Fatalf("expected forwarded client address, got %q", got)
	}
}

func TestFromRequest_TrustsProxyRealIP(t *testing.T) {
	t.Parallel()

	r := requestWith("127.0.0.1:4321", map[string]string{"X-Real-IP": testClientIP})

	if got := FromRequest(r); got != testClientIP {
		t.Fatalf("expected X-Real-IP value, got %q", got)
	}
}

func TestFromRequest_NoHeaders(t *testing.T) {
	t.Parallel()

	if got := FromRequest(requestWith("198.51.100.7:1111", nil)); got != "198.51.100.7" {
		t.Fatalf("expected peer address, got %q", got)
	}
}

// --- Edge-case coverage (BUG-005) ---

func TestFromRequest_AllTrustedXFF_FallsThroughToRealIP(t *testing.T) {
	t.Parallel()

	// Peer is 10.0.0.5 (trusted). XFF contains only trusted proxies.
	// The walker should exhaust the chain and fall through to X-Real-IP.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-For": "10.0.0.3, 10.0.0.4, 10.0.0.5",
		"X-Real-IP":       "203.0.113.99",
	})

	if got := FromRequest(r); got != "203.0.113.99" {
		t.Fatalf("expected X-Real-IP fallback, got %q", got)
	}
}

func TestFromRequest_AllTrustedXFF_FallsThroughToPeer(t *testing.T) {
	t.Parallel()

	// Peer is 127.0.0.1 (trusted). XFF contains only trusted proxies.
	// No X-Real-IP. Should fall through to the peer address.
	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Forwarded-For": "10.0.0.3, 127.0.0.1",
	})

	if got := FromRequest(r); got != "127.0.0.1" {
		t.Fatalf("expected peer fallback, got %q", got)
	}
}

func TestFromRequest_InvalidIPsInXFF_Skipped(t *testing.T) {
	t.Parallel()

	// Invalid entries in XFF should be silently skipped.
	// "not-an-ip" is skipped, "10.0.0.5" is trusted (skip), "203.0.113.50" is untrusted (return).
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-For": "not-an-ip, 203.0.113.50, 10.0.0.5",
	})

	if got := FromRequest(r); got != "203.0.113.50" {
		t.Fatalf("expected first valid untrusted IP, got %q", got)
	}
}

func TestFromRequest_IPv6_XFF(t *testing.T) {
	t.Parallel()

	// IPv6 client behind an IPv6 trusted proxy (fc00::1 is in fc00::/7).
	r := requestWith("[fc00::1]:4321", map[string]string{
		"X-Forwarded-For": "2001:db8::1, fc00::1",
	})

	if got := FromRequest(r); got != "2001:db8::1" {
		t.Fatalf("expected IPv6 client from XFF, got %q", got)
	}
}

func TestFromRequest_IPv6_RemoteAddr(t *testing.T) {
	t.Parallel()

	// Untrusted IPv6 peer, no headers → return peer.
	r := requestWith("[2001:db8::2]:4321", nil)

	if got := FromRequest(r); got != "2001:db8::2" {
		t.Fatalf("expected IPv6 peer, got %q", got)
	}
}

func TestFromRequest_CanonicalIPv6Normalization(t *testing.T) {
	t.Parallel()

	// Non-canonical IPv6 in X-Real-IP should be returned in canonical form.
	// "2001:0db8:0000:0000:0000:0000:0000:0001" canonicalizes to "2001:db8::1".
	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Real-IP": "2001:0db8:0000:0000:0000:0000:0000:0001",
	})

	if got := FromRequest(r); got != "2001:db8::1" {
		t.Fatalf("expected canonical IPv6, got %q", got)
	}
}

func TestFromRequest_CanonicalIPv4FromXFF(t *testing.T) {
	t.Parallel()

	// The XFF string should be canonicalized via net.IP.String().
	// Most IPv4 forms are already canonical, but verify the return matches ip.String().
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-For": "203.0.113.50, 10.0.0.5",
	})

	if got := FromRequest(r); got != "203.0.113.50" {
		t.Fatalf("expected canonical IPv4 from XFF, got %q", got)
	}
}

func TestFromRequest_EmptyXFF_FallsToRealIP(t *testing.T) {
	t.Parallel()

	// Empty XFF header → skip to X-Real-IP.
	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Forwarded-For": "",
		"X-Real-IP":       "203.0.113.77",
	})

	if got := FromRequest(r); got != "203.0.113.77" {
		t.Fatalf("expected X-Real-IP when XFF is empty, got %q", got)
	}
}

func TestFromRequest_MultiProxyChain(t *testing.T) {
	t.Parallel()

	// Chain: client → 10.0.0.2 (trusted) → 10.0.0.3 (trusted) → 10.0.0.5 (peer, trusted).
	// XFF: "203.0.113.10, 10.0.0.2, 10.0.0.3" (left to right: client, proxy1, proxy2).
	// Walker goes right-to-left: 10.0.0.3 (trusted, skip), 10.0.0.2 (trusted, skip),
	// 203.0.113.10 (untrusted, return).
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-For": "203.0.113.10, 10.0.0.2, 10.0.0.3",
	})

	if got := FromRequest(r); got != "203.0.113.10" {
		t.Fatalf("expected original client past multi-proxy chain, got %q", got)
	}
}

func TestFromRequest_InvalidRealIP_FallsToPeer(t *testing.T) {
	t.Parallel()

	// Trusted peer, XFF all trusted, X-Real-IP invalid → fall to peer.
	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Forwarded-For": "10.0.0.5",
		"X-Real-IP":       "not-an-ip",
	})

	if got := FromRequest(r); got != "127.0.0.1" {
		t.Fatalf("expected peer when X-Real-IP is invalid, got %q", got)
	}
}

func TestFromRequest_NoPortInRemoteAddr(t *testing.T) {
	t.Parallel()

	// RemoteAddr without a port — SplitHostPort fails, fallback to trimming.
	r := requestWith("203.0.113.20", nil)

	if got := FromRequest(r); got != "203.0.113.20" {
		t.Fatalf("expected peer without port, got %q", got)
	}
}

func TestIsTrustedProxy_NilIP(t *testing.T) {
	t.Parallel()

	if IsTrustedProxy(nil) {
		t.Fatal("expected nil IP to not be trusted")
	}
}

// --- JC_TRUSTED_PROXIES override tests (BUG-002) ---
// These tests are NOT parallel because they mutate the global trusted-proxy
// cache via resetForTesting and the JC_TRUSTED_PROXIES env var.

func TestTrustedProxies_EnvOverride(t *testing.T) {
	t.Setenv("JC_TRUSTED_PROXIES", "198.51.100.0/24")
	resetForTesting()

	defer resetForTesting()

	// 198.51.100.10 is now a trusted proxy; 10.0.0.5 is NOT (defaults replaced).
	r := requestWith("198.51.100.10:4321", map[string]string{
		"X-Forwarded-For": "203.0.113.50, 198.51.100.10",
	})

	if got := FromRequest(r); got != "203.0.113.50" {
		t.Fatalf("expected forwarded IP via custom trusted proxy, got %q", got)
	}

	// 10.0.0.5 is no longer trusted — headers should be ignored.
	r2 := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-For": "203.0.113.50",
	})

	if got := FromRequest(r2); got != "10.0.0.5" {
		t.Fatalf("expected default CIDR to be untrusted after override, got %q", got)
	}
}

func TestTrustedProxies_EnvOverride_BareIP(t *testing.T) {
	t.Setenv("JC_TRUSTED_PROXIES", "198.51.100.10")
	resetForTesting()

	defer resetForTesting()

	// Bare IP (no CIDR) — only that exact IP is trusted.
	r := requestWith("198.51.100.10:4321", map[string]string{
		"X-Real-IP": "203.0.113.60",
	})

	if got := FromRequest(r); got != "203.0.113.60" {
		t.Fatalf("expected forwarded IP via bare-IP trusted proxy, got %q", got)
	}

	// A different IP in the same /24 is NOT trusted.
	r2 := requestWith("198.51.100.11:4321", map[string]string{
		"X-Real-IP": "203.0.113.60",
	})

	if got := FromRequest(r2); got != "198.51.100.11" {
		t.Fatalf("expected non-trusted peer, got %q", got)
	}
}

func TestTrustedProxies_DefaultsRestored(t *testing.T) {
	// Ensure the cache is reset so defaults are restored for subsequent tests.
	resetForTesting()

	// 127.0.0.1 should be trusted again (default).
	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Real-IP": "203.0.113.70",
	})

	if got := FromRequest(r); got != "203.0.113.70" {
		t.Fatalf("expected default loopback to be trusted after reset, got %q", got)
	}
}

// --- Additional edge-case coverage (CIP-3) ---

func TestFromRequest_EmptyRemoteAddr_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	// RemoteAddr is empty (can happen in tests or unusual server configs).
	// No headers should be consulted because we can't verify the peer.
	r := requestWith("", map[string]string{
		"X-Forwarded-For": "203.0.113.10",
		"X-Real-IP":       "203.0.113.20",
	})

	if got := FromRequest(r); got != "" {
		t.Fatalf("expected empty string for empty RemoteAddr, got %q", got)
	}
}

func TestFromRequest_IPv4MappedIPv6_InXFF(t *testing.T) {
	t.Parallel()

	// ::ffff:203.0.113.10 is an IPv4-mapped IPv6 address. It should be
	// canonicalized to "203.0.113.10" and treated as untrusted (not in
	// any default trusted CIDR).
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-For": "::ffff:203.0.113.10, 10.0.0.5",
	})

	if got := FromRequest(r); got != "203.0.113.10" {
		t.Fatalf("expected canonical IPv4 from mapped IPv6, got %q", got)
	}
}

func TestFromRequest_IPv4MappedIPv6_Loopback_IsTrusted(t *testing.T) {
	t.Parallel()

	// ::ffff:127.0.0.1 maps to 127.0.0.1, which is in the default trusted
	// set (127.0.0.0/8). The walker should skip it and fall through to
	// X-Real-IP.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-For": "::ffff:127.0.0.1, 10.0.0.5",
		"X-Real-IP":       "203.0.113.99",
	})

	if got := FromRequest(r); got != "203.0.113.99" {
		t.Fatalf("expected mapped loopback to be trusted, fall through to X-Real-IP, got %q", got)
	}
}

func TestFromRequest_WhitespaceOnlyXFF_FallsToRealIP(t *testing.T) {
	t.Parallel()

	// XFF with only whitespace/commas → all entries trimmed to "" and
	// skipped. Should fall through to X-Real-IP.
	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Forwarded-For": " , , , ",
		"X-Real-IP":       "203.0.113.88",
	})

	if got := FromRequest(r); got != "203.0.113.88" {
		t.Fatalf("expected X-Real-IP when XFF is whitespace-only, got %q", got)
	}
}

func TestFromRequest_AllInvalidXFF_FallsToRealIP(t *testing.T) {
	t.Parallel()

	// XFF with only invalid IPs → all skipped. Fall through to X-Real-IP.
	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Forwarded-For": "not-an-ip, also-bad, 999.999.999.999",
		"X-Real-IP":       "203.0.113.77",
	})

	if got := FromRequest(r); got != "203.0.113.77" {
		t.Fatalf("expected X-Real-IP when XFF is all invalid, got %q", got)
	}
}

func TestFromRequest_AllInvalidXFF_NoRealIP_FallsToPeer(t *testing.T) {
	t.Parallel()

	// XFF all invalid, no X-Real-IP → fall through to peer.
	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Forwarded-For": "not-an-ip, also-bad",
	})

	if got := FromRequest(r); got != "127.0.0.1" {
		t.Fatalf("expected peer when XFF all invalid and no X-Real-IP, got %q", got)
	}
}

func TestTrustedProxies_EnvOverride_AllInvalid_NoTrustedProxies(t *testing.T) {
	// All-invalid JC_TRUSTED_PROXIES → no trusted proxies. Forwarding
	// headers from any peer (including default-trusted loopback) are
	// ignored. This is the safe behaviour: a garbage config yields no
	// trusted proxies, not the defaults.
	t.Setenv("JC_TRUSTED_PROXIES", "garbage, not-a-cidr, ???")
	resetForTesting()

	defer resetForTesting()

	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Real-IP": "203.0.113.50",
	})

	if got := FromRequest(r); got != "127.0.0.1" {
		t.Fatalf("expected peer (no trusted proxies after all-invalid env), got %q", got)
	}
}

func TestTrustedProxies_EnvOverride_MixedValidInvalid(t *testing.T) {
	// Mixed valid/invalid entries → valid ones are used, invalid skipped.
	t.Setenv("JC_TRUSTED_PROXIES", "garbage, 198.51.100.0/24, not-an-ip")
	resetForTesting()

	defer resetForTesting()

	// 198.51.100.10 is trusted (valid CIDR).
	r := requestWith("198.51.100.10:4321", map[string]string{
		"X-Real-IP": "203.0.113.60",
	})

	if got := FromRequest(r); got != "203.0.113.60" {
		t.Fatalf("expected forwarded IP via valid CIDR in mixed env, got %q", got)
	}

	// 127.0.0.1 is NOT trusted (defaults replaced, only 198.51.100.0/24 is).
	r2 := requestWith("127.0.0.1:4321", map[string]string{
		"X-Real-IP": "203.0.113.60",
	})

	if got := FromRequest(r2); got != "127.0.0.1" {
		t.Fatalf("expected loopback untrusted after mixed env override, got %q", got)
	}
}

func TestTrustedProxies_EnvOverride_EmptyString_UsesDefaults(t *testing.T) {
	// JC_TRUSTED_PROXIES set to empty string → os.Getenv returns "" →
	// defaults are used (same as unset). This documents the behaviour that
	// an operator cannot disable all trusted proxies by setting the env
	// var to empty — empty means "use defaults".
	t.Setenv("JC_TRUSTED_PROXIES", "")
	resetForTesting()

	defer resetForTesting()

	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Real-IP": "203.0.113.70",
	})

	if got := FromRequest(r); got != "203.0.113.70" {
		t.Fatalf("expected defaults when JC_TRUSTED_PROXIES is empty string, got %q", got)
	}
}

func TestIsTrustedProxy_IPv4MappedIPv6_Loopback(t *testing.T) {
	t.Parallel()

	// ::ffff:127.0.0.1 should be trusted because it maps to 127.0.0.1,
	// which is in 127.0.0.0/8.
	ip := net.ParseIP("::ffff:127.0.0.1")
	if ip == nil {
		t.Fatal("failed to parse ::ffff:127.0.0.1")
	}

	if !IsTrustedProxy(ip) {
		t.Fatal("expected ::ffff:127.0.0.1 to be trusted (maps to loopback)")
	}
}

// --- PeerIP / IsTrustedPeer (consumer gating helpers) ---

func TestPeerIP_ReturnsCanonicalPeerWithoutHeaders(t *testing.T) {
	t.Parallel()

	// PeerIP must ignore forwarding headers and return the TCP peer only.
	r := requestWith("203.0.113.42:4321", map[string]string{
		"X-Forwarded-For": "1.1.1.1",
		"X-Real-IP":       "2.2.2.2",
	})

	if got := PeerIP(r); got != "203.0.113.42" {
		t.Fatalf("expected peer address, got %q", got)
	}
}

func TestPeerIP_EmptyRemoteAddr(t *testing.T) {
	t.Parallel()

	if got := PeerIP(requestWith("", nil)); got != "" {
		t.Fatalf("expected empty peer, got %q", got)
	}
}

func TestIsTrustedPeer_TrustedLoopback(t *testing.T) {
	t.Parallel()

	if !IsTrustedPeer(requestWith("127.0.0.1:4321", nil)) {
		t.Fatal("expected loopback peer to be trusted")
	}
}

func TestIsTrustedPeer_UntrustedPublic(t *testing.T) {
	t.Parallel()

	if IsTrustedPeer(requestWith("203.0.113.10:4321", nil)) {
		t.Fatal("expected public peer to be untrusted")
	}
}

func TestIsTrustedPeer_EmptyRemoteAddr(t *testing.T) {
	t.Parallel()

	// Empty RemoteAddr must not be trusted (no peer to vouch for headers).
	if IsTrustedPeer(requestWith("", nil)) {
		t.Fatal("expected empty RemoteAddr to be untrusted")
	}
}

func TestIsTrustedPeer_IgnoresForwardedHeaders(t *testing.T) {
	t.Parallel()

	// An untrusted peer cannot promote itself by sending forwarding headers.
	r := requestWith("203.0.113.10:4321", map[string]string{
		"X-Forwarded-For": "127.0.0.1",
	})

	if IsTrustedPeer(r) {
		t.Fatal("expected untrusted peer to remain untrusted despite spoofed XFF")
	}
}

// --- ForwardedBaseURL / ForwardedHost / ForwardedScheme tests (host-header injection fix) ---

func TestForwardedBaseURL_TrustedProxy_UsesForwardedHost(t *testing.T) {
	t.Parallel()

	// Peer 10.0.0.5 is a trusted proxy. X-Forwarded-Host/Proto should be honoured.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Host":  "japaconnect.com",
		"X-Forwarded-Proto": "https",
	})

	if got := ForwardedBaseURL(r, "http://localhost:8080"); got != "https://japaconnect.com" {
		t.Fatalf("expected forwarded base URL from trusted proxy, got %q", got)
	}
}

func TestForwardedBaseURL_UntrustedPeer_IgnoresForwardedHost(t *testing.T) {
	t.Parallel()

	// Peer 203.0.113.10 is NOT a trusted proxy. X-Forwarded-Host must be
	// ignored — this is the host-header injection fix. Without it an
	// attacker could send X-Forwarded-Host: evil.com and get password-reset
	// / RSS links pointing to their domain.
	r := requestWith("203.0.113.10:4321", map[string]string{
		"X-Forwarded-Host":  "evil.com",
		"X-Forwarded-Proto": "https",
	})

	if got := ForwardedBaseURL(r, "https://japaconnect.com"); got != "https://japaconnect.com" {
		t.Fatalf("expected fallback (not spoofed host), got %q", got)
	}
}

func TestForwardedBaseURL_TrustedProxy_NoHeaders_ReturnsFallback(t *testing.T) {
	t.Parallel()

	// Trusted peer but no forwarding headers → fallback.
	r := requestWith("10.0.0.5:4321", nil)

	if got := ForwardedBaseURL(r, "https://configured.example.com"); got != "https://configured.example.com" {
		t.Fatalf("expected fallback when no forwarded headers, got %q", got)
	}
}

func TestForwardedBaseURL_TrustedProxy_NoProto_DefaultsToHTTP(t *testing.T) {
	t.Parallel()

	// Trusted peer, X-Forwarded-Host present but no X-Forwarded-Proto and
	// no TLS → scheme defaults to "http".
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Host": "japaconnect.com",
	})

	if got := ForwardedBaseURL(r, "https://fallback.com"); got != "http://japaconnect.com" {
		t.Fatalf("expected http scheme when no proto and no TLS, got %q", got)
	}
}

func TestForwardedHost_UntrustedPeer_ReturnsEmpty(t *testing.T) {
	t.Parallel()

	r := requestWith("8.8.8.8:4321", map[string]string{
		"X-Forwarded-Host": "evil.com",
	})

	if got := ForwardedHost(r); got != "" {
		t.Fatalf("expected empty string from untrusted peer, got %q", got)
	}
}

func TestForwardedHost_TrustedPeer_ReturnsHost(t *testing.T) {
	t.Parallel()

	r := requestWith("127.0.0.1:4321", map[string]string{
		"X-Forwarded-Host": "  japaconnect.com  ",
	})

	if got := ForwardedHost(r); got != "japaconnect.com" {
		t.Fatalf("expected trimmed forwarded host, got %q", got)
	}
}

func TestForwardedScheme_UntrustedPeer_FallsBackToTLSState(t *testing.T) {
	t.Parallel()

	// Untrusted peer with spoofed X-Forwarded-Proto: https — must be ignored.
	// No TLS → "http".
	r := requestWith("203.0.113.10:4321", map[string]string{
		"X-Forwarded-Proto": "https",
	})

	if got := ForwardedScheme(r); got != "http" {
		t.Fatalf("expected http (untrusted peer, no TLS), got %q", got)
	}
}

func TestForwardedScheme_TrustedPeer_UsesForwardedProto(t *testing.T) {
	t.Parallel()

	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Proto": "https",
	})

	if got := ForwardedScheme(r); got != "https" {
		t.Fatalf("expected https from trusted proxy forwarded proto, got %q", got)
	}
}

// --- Forwarded header value sanitization (CIP-4) ---
//
// ForwardedScheme and ForwardedHost gate on the trusted-proxy peer, but the
// header VALUE is still client-influenced (a misconfigured or compromised
// proxy, or an operator who sets JC_TRUSTED_PROXIES too broadly, lets an
// attacker control it). Because these values are concatenated into public
// URLs — RSS <link> elements, password-reset email links — they must be
// validated, not just gated. Otherwise a trusted peer can inject
// "javascript:" schemes, CRLF sequences, or path separators into URLs.

func TestForwardedScheme_RejectsNonHTTPScheme(t *testing.T) {
	t.Parallel()

	// A "javascript:" scheme must never be returned — it would produce a
	// javascript:// link in a password-reset email or RSS feed. Fall back
	// to the TLS state (no TLS here → "http").
	for _, proto := range []string{"javascript:alert(1)//", "ftp", "file", "gopher", "htt p"} {
		r := requestWith("10.0.0.5:4321", map[string]string{
			"X-Forwarded-Proto": proto,
		})

		if got := ForwardedScheme(r); got != "http" {
			t.Fatalf("proto %q: expected http fallback, got %q", proto, got)
		}
	}
}

func TestForwardedScheme_NormalizesCase(t *testing.T) {
	t.Parallel()

	// "HTTPS" and "Http" are valid schemes; return them lowercased so the
	// resulting URL is canonical and downstream string comparisons hold.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Proto": "HTTPS",
	})

	if got := ForwardedScheme(r); got != "https" {
		t.Fatalf("expected lowercased https, got %q", got)
	}
}

func TestForwardedScheme_RejectsCRLFInjection(t *testing.T) {
	t.Parallel()

	// CRLF in the proto could split an HTTP header or RSS XML line.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Proto": "https\r\nX-Injected: evil",
	})

	if got := ForwardedScheme(r); got != "http" {
		t.Fatalf("expected http fallback for CRLF proto, got %q", got)
	}
}

func TestForwardedHost_RejectsCRLFInjection(t *testing.T) {
	t.Parallel()

	// CRLF in the host could inject headers / split RSS XML when the value
	// is concatenated into a URL. Must return "" so the caller falls back.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Host": "japaconnect.com\r\nBcc: attacker@evil.com",
	})

	if got := ForwardedHost(r); got != "" {
		t.Fatalf("expected empty host for CRLF injection, got %q", got)
	}
}

func TestForwardedHost_RejectsPathSeparator(t *testing.T) {
	t.Parallel()

	// A host with a path separator is not a bare host — it would turn
	// "base URL" into a URL with an attacker-controlled path component.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Host": "evil.com/reset?token=stolen",
	})

	if got := ForwardedHost(r); got != "" {
		t.Fatalf("expected empty host for path-bearing host, got %q", got)
	}
}

func TestForwardedHost_RejectsEmbeddedWhitespace(t *testing.T) {
	t.Parallel()

	// Internal whitespace (not just leading/trailing) breaks URL parsing.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Host": "evil.com other.com",
	})

	if got := ForwardedHost(r); got != "" {
		t.Fatalf("expected empty host for embedded whitespace, got %q", got)
	}
}

func TestForwardedHost_AcceptsPortBearingHost(t *testing.T) {
	t.Parallel()

	// A legitimate host with an explicit port must still be accepted.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Host": "japaconnect.com:8443",
	})

	if got := ForwardedHost(r); got != "japaconnect.com:8443" {
		t.Fatalf("expected port-bearing host, got %q", got)
	}
}

func TestForwardedHost_AcceptsIPv6BracketedHost(t *testing.T) {
	t.Parallel()

	// IPv6 host with port uses bracketed form: [::1]:8080.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Host": "[::1]:8080",
	})

	if got := ForwardedHost(r); got != "[::1]:8080" {
		t.Fatalf("expected bracketed IPv6 host, got %q", got)
	}
}

func TestForwardedBaseURL_RejectsJavaScriptScheme(t *testing.T) {
	t.Parallel()

	// Defense in depth: even from a trusted peer, a "javascript:" proto
	// must not produce a javascript:// URL that ends up in an email link.
	// The scheme is rejected and falls back to "http" (no TLS); the trusted
	// host is still honoured, yielding a plain http:// URL — the
	// javascript:// link-poisoning vector is neutralized.
	r := requestWith("10.0.0.5:4321", map[string]string{
		"X-Forwarded-Host":  "evil.com",
		"X-Forwarded-Proto": "javascript:alert(1)//",
	})

	if got := ForwardedBaseURL(r, "https://japaconnect.com"); got != "http://evil.com" {
		t.Fatalf("expected http://evil.com (javascript scheme rejected), got %q", got)
	}
}
