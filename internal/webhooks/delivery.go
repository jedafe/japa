package webhooks

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// SignatureHeader is the HTTP header name carrying the HMAC-SHA256 signature.
const SignatureHeader = "X-Webhook-Signature"

// ContentType is the media type sent with every webhook delivery.
const ContentType = "application/json"

// maxResponseCaptureBytes limits how much of the webhook response body is
// stored in last_response_body for debugging (§12). 4 KiB is enough to capture
// a meaningful error message without unbounded storage.
const maxResponseCaptureBytes = 4096

// ssrfDialContext is the production dial function that rejects private/
// loopback/link-local IP ranges to prevent SSRF (BUG-014). Tests replace
// this with an unrestricted dialer so httptest.Server (which binds to
// 127.0.0.1) can be reached.
//
// The dial-time Control hook re-validates the connection IP: the pre-flight
// LookupIP check below can race a DNS rebinding (validate public, dial
// private), so the kernel-resolved address is checked again at connect
// time — the same defense as linkpreview's newSSRFSafeClient.
var ssrfDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %s: %w", addr, err)
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", host, err)
	}

	for _, ip := range ips {
		if isBlockedIP(ip.IP) {
			return nil, fmt.Errorf("SSRF blocked: %s resolves to private/reserved IP %s", host, ip.IP)
		}
	}

	d := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			h, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if ip := net.ParseIP(h); ip != nil && isBlockedIP(ip) {
				return fmt.Errorf("SSRF blocked at connect time: %s", ip)
			}
			return nil
		},
	}
	return d.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// deliveryClient is a hardened HTTP client for outbound webhook delivery
// (SEC-009). It enforces a 30s timeout, caps redirects at 5, blocks redirects
// to non-http(s) schemes, and uses a dialer that rejects private/loopback/
// link-local IP ranges to prevent SSRF (BUG-014). http.DefaultClient has no
// timeout, unlimited redirects, and no IP filtering — users could register
// internal-IP webhook URLs and exfiltrate data or scan the internal network.
var deliveryClient = newDeliveryClient(ssrfDialContext)

// newDeliveryClient builds an HTTP client with the given dial function.
func newDeliveryClient(dial func(ctx context.Context, network, addr string) (net.Conn, error)) *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			// Re-validate every redirect hop: scheme, host, and userinfo.
			// A 302 from a public URL to http://169.254.169.254/ must be
			// refused before following, not merely caught at dial time.
			if err := validateEndpoint(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
		Transport: &http.Transport{DialContext: dial},
	}
}

// isBlockedIP reports whether the IP is in a private, loopback, link-local,
// or reserved range that must not be contacted for outbound webhook delivery
// (SSRF defense, BUG-014).
func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	if ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	// IPv4-mapped IPv6 — check the underlying IPv4.
	if v4 := ip.To4(); v4 != nil {
		// 169.254.0.0/16 (link-local) is covered by IsLinkLocalUnicast for IPv4.
		// 100.64.0.0/10 (CGNAT / shared address space) — not covered by stdlib.
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
		// 0.0.0.0/8 (current network) — IsUnspecified covers only all-zeros.
		if v4[0] == 0 {
			return true
		}
		// 192.0.0.0/24 (IETF protocol assignments).
		if v4[0] == 192 && v4[1] == 0 && v4[2] == 0 {
			return true
		}
		// 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24 (documentation).
		if (v4[0] == 192 && v4[1] == 0 && v4[2] == 2) ||
			(v4[0] == 198 && v4[1] == 51 && v4[2] == 100) ||
			(v4[0] == 203 && v4[1] == 0 && v4[2] == 113) {
			return true
		}
		// 198.18.0.0/15 (benchmarking).
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return true
		}
		// 240.0.0.0/4 (reserved, includes 255.255.255.255 broadcast).
		if v4[0] >= 240 {
			return true
		}
	}

	return false
}

// validateEndpoint checks that the webhook endpoint URL is safe to call
// (SEC-009, BUG-014). Rejects non-http(s) schemes which would allow
// javascript:/data:/file: URLs, empty hosts, and embedded userinfo
// (credentials in the URL leak into logs and error messages).
func validateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid webhook endpoint: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook endpoint scheme must be http or https, got: %s", u.Scheme)
	}

	if u.Host == "" {
		return errors.New("webhook endpoint has empty host")
	}

	if u.User != nil {
		return errors.New("webhook endpoint must not embed userinfo credentials")
	}

	return nil
}

// DeliveryResult captures the outcome of a single delivery attempt for the
// delivery log (§12 last_response_code / last_response_body).
type DeliveryResult struct {
	StatusCode   int
	ResponseBody string
}

// Deliver sends a single HTTP POST to the endpoint with the signed envelope.
// Returns an error if the request fails or the endpoint returns a non-2xx
// status code. On a non-2xx response, the status code and a truncated response
// body are still captured in the DeliveryResult for the delivery log.
func Deliver(ctx context.Context, endpoint, secret string, envelope WebhookEnvelope) (DeliveryResult, error) {
	result := DeliveryResult{}

	if err := validateEndpoint(endpoint); err != nil {
		return result, err
	}

	payload, err := envelope.Marshal()
	if err != nil {
		return result, fmt.Errorf("marshaling webhook envelope: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return result, fmt.Errorf("creating webhook request: %w", err)
	}

	req.Header.Set("Content-Type", ContentType)
	req.Header.Set(SignatureHeader, SignPayload(secret, payload))

	resp, err := deliveryClient.Do(req)
	if err != nil {
		return result, fmt.Errorf("sending webhook: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	result.StatusCode = resp.StatusCode

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseCaptureBytes))
	result.ResponseBody = string(body)

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return result, nil
}

// DeliverWithRetry calls Deliver up to maxRetries times with exponential
// backoff. The delay before retry attempt N (0-indexed) is 2^N seconds,
// producing the schedule: 1s, 2s, 4s, 8s, 16s, ...  Returns nil on the first
// successful delivery, or the last error if all attempts fail. The final
// DeliveryResult (from the last attempt) is returned alongside the error so
// the caller can record last_response_code / last_response_body.
func DeliverWithRetry(ctx context.Context, endpoint, secret string, envelope WebhookEnvelope, maxRetries int) (DeliveryResult, error) {
	if maxRetries < 1 {
		maxRetries = 1
	}

	// Cap retries to prevent integer overflow in exponential backoff calculation.
	// 2^30 seconds ≈ 34 years — any reasonable retry count is well below this.
	if maxRetries > 30 {
		maxRetries = 30
	}

	var lastErr error

	var lastResult DeliveryResult

	for attempt := range maxRetries {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second //nolint:gosec // G115: attempt bounded by maxRetries cap (30)
			select {
			case <-ctx.Done():
				return lastResult, ctx.Err()
			case <-time.After(delay):
			}
		}

		lastResult, lastErr = Deliver(ctx, endpoint, secret, envelope)
		if lastErr == nil {
			return lastResult, nil
		}

		slog.Warn("webhook delivery attempt failed",
			"endpoint", endpoint,
			"attempt", attempt+1,
			"max_retries", maxRetries,
			"status_code", lastResult.StatusCode,
			"error", lastErr)
	}

	return lastResult, fmt.Errorf("webhook delivery failed after %d attempts: %w", maxRetries, lastErr)
}

// PerformChallenge sends a GET request to {endpoint}?challenge={token} and
// verifies that the response body contains the token. This is the §12
// challenge/response endpoint verification that must succeed before a webhook
// subscription is activated (BUG-001).
func PerformChallenge(ctx context.Context, endpoint, token string) error {
	if err := validateEndpoint(endpoint); err != nil {
		return err
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parsing endpoint for challenge: %w", err)
	}

	q := u.Query()
	q.Set("challenge", token)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("creating challenge request: %w", err)
	}

	resp, err := deliveryClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending challenge: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("challenge returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseCaptureBytes))
	if err != nil {
		return fmt.Errorf("reading challenge response: %w", err)
	}

	// The webhook must echo the token back in the response body. Trim
	// whitespace to allow trailing newlines.
	if strings.TrimSpace(string(body)) != token {
		return errors.New("challenge response mismatch: endpoint did not echo the challenge token")
	}

	return nil
}
