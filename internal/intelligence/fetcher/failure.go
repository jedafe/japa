package fetcher

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FailureClass buckets a fetch failure for the worker's retry and
// circuit-breaker policy — the taxonomy that decides whether a failure
// counts against the channel's consecutive_failures.
type FailureClass int

const (
	// FailureTransient is a retryable failure (network error, timeout,
	// 5xx, adapter internals). It must not increment consecutive_failures
	// — a flap cannot burn a channel toward auto-disable; the task's own
	// retry policy handles it.
	FailureTransient FailureClass = iota
	// FailureThrottled is an upstream HTTP 429 — retryable, but only after
	// the site's Retry-After window; the worker stamps a domain cool-off
	// so sibling channels on the host don't stampede.
	FailureThrottled
	// FailureBlocked means the site actively refused the crawl: a bot-wall
	// challenge page or a bare access denial. Escalation (implicit
	// render/wayback ladder hops in Composite.Fetch) is the designed
	// response — neither retries nor disable pressure apply.
	FailureBlocked
	// FailureGone is a permanent absence (404/410, other 4xx, DNS
	// NXDOMAIN) — counted toward auto-disable at the tighter gone
	// threshold; retrying mid-flight cannot help.
	FailureGone
)

var (
	// ErrBotWalled marks a response that is an anti-bot challenge rather
	// than content — detected on any status, including deceptive 200s.
	ErrBotWalled = errors.New("fetcher: bot wall or anti-bot challenge")

	// ErrAccessDenied is a bare 401/403 with no challenge signature —
	// access-controlled or WAF-blocked without a solvable page.
	ErrAccessDenied = errors.New("fetcher: access denied")

	// ErrUpstreamThrottled marks an upstream HTTP 429. Distinct from
	// ErrRateLimited (our self-imposed per-domain budget) — this one is
	// the SITE telling the crawler to slow down.
	ErrUpstreamThrottled = errors.New("fetcher: upstream rate limited")

	// ErrContentGone marks a permanently missing resource (404/410,
	// other 4xx request-shape failures, DNS NXDOMAIN).
	ErrContentGone = errors.New("fetcher: upstream resource gone")

	// ErrContentCollapsed is raised by the worker-side poison guard —
	// never returned by a fetch adapter, since collapse is only visible
	// against a prior baseline. A top-level HTML capture that shrank past
	// the collapse bounds is a suspected wall variant and is excluded
	// from raw-doc persistence and extraction.
	ErrContentCollapsed = errors.New("fetcher: content collapsed vs prior capture (suspected bot wall)")
)

// ThrottledError carries the upstream's Retry-After hint on HTTP 429 so
// the worker can size the domain cool-off from what the site asked for
// rather than a flat default.
type ThrottledError struct {
	Method     string
	RetryAfter time.Duration
}

func (e *ThrottledError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s: upstream rate limited (retry-after %s)", e.Method, e.RetryAfter)
	}

	return e.Method + ": upstream rate limited"
}

// Is matches ErrUpstreamThrottled through errors.Is — the typed error
// carries data, so it cannot BE the sentinel.
func (e *ThrottledError) Is(target error) bool { return target == ErrUpstreamThrottled }

// WallError records which wall signature fired and on what status — the
// detail the channel's last_error_message surfaces for ops triage.
type WallError struct {
	Method    string
	Status    int
	Signature string
}

func (e *WallError) Error() string {
	return fmt.Sprintf("%s: bot wall detected (signature %s, status %d)", e.Method, e.Signature, e.Status)
}

// Is matches ErrBotWalled through errors.Is.
func (e *WallError) Is(target error) bool { return target == ErrBotWalled }

// ClassifyFetchError maps a fetch failure to its FailureClass. Sentinels
// carry the classification; a DNS not-found counts as gone; everything
// else — transport errors, timeouts, 5xx, adapter internals — is
// transient by default, the safe bucket: retries may help and no disable
// pressure accrues.
func ClassifyFetchError(err error) FailureClass {
	switch {
	case errors.Is(err, ErrBotWalled), errors.Is(err, ErrAccessDenied),
		errors.Is(err, ErrContentCollapsed):
		return FailureBlocked
	case errors.Is(err, ErrUpstreamThrottled):
		return FailureThrottled
	case errors.Is(err, ErrContentGone):
		return FailureGone
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return FailureGone
	}

	return FailureTransient
}

// httpFailureError builds the classified error for a >=400 response. Wall
// signatures are checked on the error body first — vendors answer
// challenges with 403/503, so status alone cannot tell "denied because
// WAF" from "denied because gone".
func httpFailureError(method string, resp *http.Response, body []byte, contentType string) error {
	status := resp.StatusCode

	if status == http.StatusTooManyRequests {
		return &ThrottledError{Method: method, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}

	if sig, ok := DetectWall(status, resp.Header, body, contentType); ok {
		return &WallError{Method: method, Status: status, Signature: sig}
	}

	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%s: %w: HTTP %d", method, ErrAccessDenied, status)
	case status >= 400 && status < 500:
		// 404/410 and every other 4xx are request-shape failures —
		// permanent until the channel config changes, so they class with
		// gone: counted at the tighter disable threshold, no retry burn.
		return fmt.Errorf("%s: %w: HTTP %d", method, ErrContentGone, status)
	default:
		return fmt.Errorf("%s: unexpected HTTP status %d", method, status)
	}
}

// parseRetryAfter reads the Retry-After header — delta-seconds or an
// HTTP date — and returns 0 when absent or unparseable.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}

	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}

	if t, err := http.ParseTime(v); err == nil {
		return time.Until(t)
	}

	return 0
}
