package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/linkpreview"
)

const (
	// defaultProbeRateLimit is the per-user hourly preview-probe quota used
	// when the intel.probe.rate_limit_per_hr setting is absent
	// (intel-channels design §10: default 20).
	defaultProbeRateLimit = 20
	// probeQuotaWindow is the fixed window the per-user probe quota covers.
	probeQuotaWindow = time.Hour
)

// errPrivateProbeTarget marks a candidate URL whose host is (or resolves to)
// a private/reserved IP — rejected before any outbound request is made.
var errPrivateProbeTarget = errors.New("probe: url targets a private/reserved address")

// probeSourceRequest is the POST /api/v1/sources/probe body. `url` is the
// canonical field; `source_url` is accepted as an alias so the propose form
// can reuse its payload field name (spec §6: {name, source_url, ...}).
type probeSourceRequest struct {
	URL        string   `json:"url"`
	SourceURL  string   `json:"source_url"`
	SourceKind string   `json:"source_kind"`
	Categories []string `json:"categories"`
}

// HandleProbeSource runs the bounded single-shot preview probe over a
// candidate source URL and returns the ranked probe report (detected
// methods, channel proposals, robots verdict). Powers the live
// detected-method chips in the propose form (intel-channels design §2).
//
// POST /api/v1/sources/probe — authenticated, per-user hourly quota via
// Valkey (jc:intel:probe_quota:{user_id}).
func (h *Handlers) HandleProbeSource(w http.ResponseWriter, r *http.Request) {
	if h.prober == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "intel_unavailable", "Source probing is not configured")
		return
	}

	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	var req probeSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}

	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" {
		rawURL = strings.TrimSpace(req.SourceURL)
	}
	if rawURL == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_url", "url is required")
		return
	}

	normalized, err := validateProbeTarget(rawURL)
	if err != nil {
		switch {
		case errors.Is(err, errPrivateProbeTarget):
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "url_not_allowed", "URL targets a private or reserved address")
		case errors.Is(err, errpkg.ErrRequired):
			errpkg.WriteError(w, http.StatusBadRequest, "missing_url", "url is required")
		default:
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_url", "url must be a valid http/https URL")
		}
		return
	}

	// Per-user fixed-window quota (spec §2 preview stage): INCR the hourly
	// counter and EXPIRE on the first increment so the window self-cleans.
	// Valkey errors fail open — a quota-store outage should not take the
	// endpoint down; the prober's SSRF guard is independent of the quota.
	if h.rdb != nil {
		limit := defaultProbeRateLimit
		if h.settings != nil {
			limit = h.settings.GetIntOr(r.Context(), "intel.probe.rate_limit_per_hr", defaultProbeRateLimit)
		}

		key := "jc:intel:probe_quota:" + userID.String()
		count, incrErr := h.rdb.Incr(r.Context(), key).Result()
		if incrErr != nil {
			slog.Warn("intel: probe quota increment failed; allowing request", "user_id", userID.String(), "error", incrErr)
		} else {
			if count == 1 {
				h.rdb.Expire(r.Context(), key, probeQuotaWindow)
			}
			if count > int64(limit) {
				errpkg.WriteError(w, http.StatusTooManyRequests, "probe_quota_exceeded", "Hourly probe limit reached; try again later")
				return
			}
		}
	}

	report, err := h.prober.Preview(r.Context(), normalized, req.SourceKind, req.Categories)
	if err != nil {
		switch {
		case errors.Is(err, errpkg.ErrRequired), errors.Is(err, errpkg.ErrInvalid):
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_url", "url must be a valid http/https URL")
		case strings.Contains(err.Error(), "SSRF"):
			// The injected safetransport rejected a private redirect target.
			errpkg.WriteError(w, http.StatusUnprocessableEntity, "url_not_allowed", "URL targets a private or reserved address")
		default:
			slog.Error("intel: source probe failed", "error", err)
			errpkg.WriteError(w, http.StatusBadGateway, "probe_failed", "Could not reach the candidate URL")
		}
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": report})
}

// validateProbeTarget normalizes and validates the candidate URL before any
// outbound request: scheme optional (https assumed), http(s) required, and
// the literal or resolved host must not be a private/reserved IP.
//
// This is the entry-URL complement of the fetcher safetransport's
// redirect-time SSRF guard (checkRedirectURL only fires on redirects — the
// first request would otherwise dial a private host unchecked). Hostname
// resolution failures fail open: the probe's own fetch surfaces unreachable
// hosts as errors, and the redirect guard still covers redirects into
// private ranges.
func validateProbeTarget(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%w: url", errpkg.ErrRequired)
	}

	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%w: url %q", errpkg.ErrInvalid, raw)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("%w: url scheme %q must be http(s)", errpkg.ErrInvalid, u.Scheme)
	}

	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if linkpreview.IsPrivateIP(ip) {
			return "", errPrivateProbeTarget
		}

		return raw, nil
	}

	// Hostname: best-effort resolution — a private address among the
	// records rejects the target outright.
	if addrs, lookupErr := net.LookupIP(host); lookupErr == nil {
		for _, a := range addrs {
			if linkpreview.IsPrivateIP(a) {
				return "", errPrivateProbeTarget
			}
		}
	}

	return raw, nil
}
