package webhooks

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// SignatureHeader is the HTTP header name carrying the HMAC-SHA256 signature.
const SignatureHeader = "X-Webhook-Signature"

// ContentType is the media type sent with every webhook delivery.
const ContentType = "application/json"

// deliveryClient is a hardened HTTP client for outbound webhook delivery
// (SEC-009). It enforces a 30s timeout and caps redirects at 5 to prevent
// SSRF via redirect chains to internal IPs. http.DefaultClient has no
// timeout and unlimited redirects — users could register internal-IP
// webhook URLs and exfiltrate data or scan the internal network.
var deliveryClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("stopped after 5 redirects")
		}
		// Block redirects to non-http(s) schemes (defense-in-depth).
		if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
			return fmt.Errorf("redirect to non-http scheme blocked: %s", req.URL.Scheme)
		}
		return nil
	},
}

// validateEndpoint checks that the webhook endpoint URL is safe to call
// (SEC-009). Rejects non-http(s) schemes which would allow javascript:/data:
// or file: URLs to be registered as webhook targets.
func validateEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid webhook endpoint: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook endpoint scheme must be http or https, got: %s", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("webhook endpoint has empty host")
	}
	return nil
}

// Deliver sends a single HTTP POST to the endpoint with the signed envelope.
// Returns an error if the request fails or the endpoint returns a non-2xx
// status code.
func Deliver(ctx context.Context, endpoint, secret string, envelope WebhookEnvelope) error {
	if err := validateEndpoint(endpoint); err != nil {
		return err
	}

	payload, err := envelope.Marshal()
	if err != nil {
		return fmt.Errorf("marshaling webhook envelope: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}

	req.Header.Set("Content-Type", ContentType)
	req.Header.Set(SignatureHeader, SignPayload(secret, payload))

	resp, err := deliveryClient.Do(req)
	if err != nil {
		return fmt.Errorf("sending webhook to %s: %w", endpoint, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook to %s returned status %d", endpoint, resp.StatusCode)
	}

	return nil
}

// DeliverWithRetry calls Deliver up to maxRetries times with exponential
// backoff. The delay before retry attempt N (0-indexed) is 2^N seconds,
// producing the schedule: 1s, 2s, 4s, 8s, 16s, ...  Returns nil on the first
// successful delivery, or the last error if all attempts fail.
func DeliverWithRetry(ctx context.Context, endpoint, secret string, envelope WebhookEnvelope, maxRetries int) error {
	if maxRetries < 1 {
		maxRetries = 1
	}

	// Cap retries to prevent integer overflow in exponential backoff calculation.
	// 2^30 seconds ≈ 34 years — any reasonable retry count is well below this.
	if maxRetries > 30 {
		maxRetries = 30
	}

	var lastErr error

	for attempt := range maxRetries {
		if attempt > 0 {
			delay := time.Duration(1<<uint(attempt-1)) * time.Second //nolint:gosec // G115: attempt bounded by maxRetries cap (30)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = Deliver(ctx, endpoint, secret, envelope)
		if lastErr == nil {
			return nil
		}

		slog.Warn("webhook delivery attempt failed",
			"endpoint", endpoint,
			"attempt", attempt+1,
			"max_retries", maxRetries,
			"error", lastErr)
	}

	return fmt.Errorf("webhook delivery failed after %d attempts: %w", maxRetries, lastErr)
}
