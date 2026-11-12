package email

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// httpClient is the shared client for vendor API providers. Vendor send
// endpoints are called synchronously inside a task that already has its own
// timeout budget (30s) — 15s keeps a slow provider from eating all of it
// before the dispatcher can try the next one.
var httpClient = &http.Client{Timeout: 15 * time.Second}

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

	internal := isInternalEmailHost(host)
	if internal && (u.Scheme == "https" || !isLoopbackEmailHost(host)) {
		return errInvalidEndpoint
	}

	return nil
}

// isInternalEmailHost reports whether host is loopback, private, or
// link-local — never a valid email vendor endpoint.
func isInternalEmailHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsUnspecified() || ip.IsMulticast()
}

// isLoopbackEmailHost reports whether host is strictly loopback.
func isLoopbackEmailHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
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
