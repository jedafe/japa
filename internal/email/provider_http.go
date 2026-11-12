package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// httpClient is the shared client for vendor API providers. Vendor send
// endpoints are called synchronously inside a task that already has its own
// timeout budget (30s) — 15s keeps a slow provider from eating all of it
// before the dispatcher can try the next one.
var httpClient = newGuardedHTTPClient()

// newGuardedHTTPClient builds the vendor client on a transport whose dialer
// rejects connections to non-loopback internal addresses AFTER connect —
// validateEmailEndpoint checks the literal host, but a hostname can resolve
// (or re-resolve) to a private IP; checking the actual remote address closes
// that DNS-rebinding hole. Loopback stays allowed because http://loopback
// base URLs are a supported local-dev relay configuration.
func newGuardedHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone() //nolint:forcetypeassert // stdlib guarantees *http.Transport
	transport.DialContext = guardedDialContext
	transport.DialTLSContext = guardedDialTLSContext

	return &http.Client{Timeout: 15 * time.Second, Transport: transport}
}

var endpointDialer = &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

// proxySchemeDefaultPorts mirrors net/http's canonicalAddr port map for
// proxy URLs — the transport dials the proxy host on this port when the
// URL omits one.
var proxySchemeDefaultPorts = map[string]string{
	"http":   "80",
	"https":  "443",
	"socks5": "1080",
}

// isProxyEndpointAddr reports whether the dialed addr is a configured
// egress proxy. Only then is the remote-IP guard skipped — the connection
// target is the deployment-owned proxy whose egress policy applies, and an
// internal proxy would otherwise be misclassified as an internal endpoint.
// A proxy env var being set does NOT mean this connection traverses it:
// scheme mismatch (HTTPS_PROXY set, request is http), NO_PROXY exclusions,
// and malformed proxy URLs all dial the target directly — skipping the
// check on env-presence alone would let an internal hostname connect
// unguarded. Host AND port must match the proxy URL (with its scheme
// default when omitted): a target sharing the proxy's hostname on a
// different port is not proxy traffic and stays guarded.
func isProxyEndpointAddr(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return false
	}

	for _, env := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		raw := os.Getenv(env)
		if raw == "" {
			continue
		}

		u, err := url.Parse(raw)
		if err != nil {
			continue
		}

		// The transport dials canonicalAddr(proxyURL) — host plus the
		// scheme-default port when the URL omits one.
		proxyPort := u.Port()
		if proxyPort == "" {
			proxyPort = proxySchemeDefaultPorts[strings.ToLower(u.Scheme)]
		}

		if strings.EqualFold(u.Hostname(), host) && proxyPort == port {
			return true
		}
	}

	return false
}

// guardedDialContext dials then inspects the connected remote address —
// the IP the request will actually reach, immune to DNS rebinding between
// validation and dial. Loopback is exempt for plain HTTP (local dev relays
// are a supported configuration).
func guardedDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := endpointDialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	if isProxyEndpointAddr(addr) {
		return conn, nil
	}

	if ta, ok := conn.RemoteAddr().(*net.TCPAddr); ok && isBlockedRemoteIP(ta.IP) {
		_ = conn.Close()

		return nil, errInvalidEndpoint
	}

	return conn, nil
}

// guardedDialTLSContext handles non-proxied HTTPS connections. Unlike the
// plain-HTTP path, loopback is NOT exempt — the URL policy already rejects
// https+loopback literal hosts, and this check makes a hostname that
// resolves to loopback fail the same way at connect time. The returned
// conn must be past the TLS handshake (http.Transport.DialTLSContext
// contract), so the handshake is performed here.
func guardedDialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := endpointDialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	if ta, ok := conn.RemoteAddr().(*net.TCPAddr); ok && isInternalEmailIP(ta.IP) {
		_ = conn.Close()

		return nil, errInvalidEndpoint
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		_ = conn.Close()

		return nil, err
	}

	tlsConn := tls.Client(conn, &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1"},
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()

		return nil, err
	}

	return tlsConn, nil
}

// isBlockedRemoteIP reports whether a connected remote IP is internal but
// not loopback — same internal classification as isInternalEmailIP, with
// the loopback exemption for local-dev relays.
func isBlockedRemoteIP(ip net.IP) bool {
	if ip.IsLoopback() {
		return false
	}

	return isInternalEmailIP(ip)
}

// errInvalidEndpoint is returned when a provider base URL fails validation.
var errInvalidEndpoint = errors.New("email: invalid or disallowed endpoint URL")

// validateEmailEndpoint validates an admin-supplied provider base URL.
// HTTPS is required for non-local hosts; HTTP is permitted only to loopback
// addresses (local dev relays). Internal/loopback hosts are rejected for
// HTTPS so a misconfigured or malicious config cannot SSRF internal
// services (defense-in-depth mirror of the AI gateway's endpoint policy).
func validateEmailEndpoint(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return errInvalidEndpoint
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return errInvalidEndpoint
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return errInvalidEndpoint
	}

	host := u.Hostname()
	if host == "" {
		return errInvalidEndpoint
	}

	// http is permitted ONLY to loopback (local dev relays) — a public
	// http:// base_url would send vendor credentials over cleartext.
	// https rejects all internal hosts incl. loopback. Mirrors the AI
	// gateway's endpoint policy.
	if u.Scheme == "http" && !isLoopbackEmailHost(host) {
		return errInvalidEndpoint
	}

	if u.Scheme == "https" && isInternalEmailHost(host) {
		return errInvalidEndpoint
	}

	return nil
}

// isInternalEmailHost reports whether host is loopback, private, or
// link-local — never a valid email vendor endpoint. Alt-IP forms
// (decimal 2130706433, hex 0x7f.0.0.1, octal 0177.0.0.1) bypass
// net.ParseIP and are normalized first — mirrors ai.isInternalHost.
func isInternalEmailHost(host string) bool {
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

	return isInternalEmailIP(ip)
}

// isInternalEmailIP reports whether an IP is loopback, private, link-local,
// unspecified, multicast, or in a reserved range the stdlib predicates miss
// (CGNAT 100.64/10, 0.x, ≥240.x, TEST-NET documentation ranges, 198.18/19
// benchmarking). Mirrors ai.isInternalIP so both packages enforce the same
// internal classification at URL validation and at dial.
func isInternalEmailIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}

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

// isLoopbackEmailHost reports whether host is strictly loopback.
func isLoopbackEmailHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		ip = normalizeNumericIP(host)
	}

	return ip != nil && ip.IsLoopback()
}

// normalizeNumericIP parses decimal/hex/octal IPv4 forms that net.ParseIP
// rejects but the dialer accepts: "2130706433" (127.0.0.1),
// "0x7f.0.0.1", "0177.0.0.1". Returns nil when the host is not a numeric
// IPv4 form. Mirrors ai.normalizeNumericIP so both packages enforce the
// same alt-IP policy — without it, https://2130706433 would reach the
// loopback-exempt dialer as an unclassified "hostname".
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

		return net.IPv4(byte(n>>24), byte(n>>16), byte(n>>8), byte(n)) // #nosec G115 -- n is ParseUint-bounded to 32 bits
	}

	var b [4]byte

	for i, p := range parts {
		n, ok := parsePart(p, 8)
		if !ok {
			return nil
		}

		b[i] = byte(n) // #nosec G115 -- n is ParseUint-bounded to 8 bits
	}

	return net.IPv4(b[0], b[1], b[2], b[3])
}

// postJSON marshals body, POSTs it to rawURL with the given headers, and
// returns the status code plus response body. Transport errors and non-2xx
// responses are both returned as errors so the dispatcher falls through to
// the next provider; the response body is included in the error for
// operator debugging (vendor APIs return useful failure detail).
func postJSON(ctx context.Context, rawURL string, headers map[string]string, body any) (int, []byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, nil, fmt.Errorf("marshal request: %w", err)
	}

	if err := validateEmailEndpoint(rawURL); err != nil {
		return 0, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(payload)) // #nosec G704 -- rawURL validated above
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, respBody, fmt.Errorf("provider returned %d: %s", resp.StatusCode, truncateForLog(string(respBody)))
	}

	return resp.StatusCode, respBody, nil
}

// truncateForLog bounds error detail embedded in provider-failure errors.
func truncateForLog(s string) string {
	const max = 300
	if len(s) > max {
		return s[:max] + "…"
	}

	return s
}

// defaultBaseURL returns the configured base URL, or the vendor default
// when unset. Trailing slashes are trimmed so callers can concatenate paths.
func defaultBaseURL(configured, fallback string) string {
	if strings.TrimSpace(configured) == "" {
		return fallback
	}

	return strings.TrimRight(configured, "/")
}
