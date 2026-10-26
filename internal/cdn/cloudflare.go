package cdn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// Cloudflare cache purge API. Bearer-token auth against
// https://api.cloudflare.com/client/v4/zones/{zone}/purge_cache.
// Body fields: files (URLs, max 30/request), tags (Enterprise plans only),
// prefixes (Enterprise), hosts, or purge_everything.
// Var (not const) so tests can point the client at a stub server.
var cfAPIBase = "https://api.cloudflare.com/client/v4"

// cfFileChunk is Cloudflare's per-request URL purge limit.
const cfFileChunk = 30

// errCFRejected marks an API-level purge rejection (HTTP request succeeded,
// success:false) — distinct from transport/decode failures so callers can
// tell a plan limitation from a flake.
var errCFRejected = errors.New("cdn: cloudflare purge rejected")

// cfTagPurgeUnsupported flips after the first tag-purge rejection: tag purge
// requires an Enterprise zone, and on every other plan each publish would
// otherwise burn a doomed API call and log a warning. File purges keep
// running; the flag only suppresses the tags call until cfTagRetryAt.
var cfTagPurgeUnsupported atomic.Bool

// cfTagRetryAt is the unix timestamp at which tag purge is re-attempted
// after a rejection — a zone upgraded to Enterprise (or given a token with
// tag-purge permission) recovers automatically after the cooldown, without
// a restart or a zone change.
var cfTagRetryAt atomic.Int64

// cfTagRetryCooldown bounds tag-purge suppression after an API-level
// rejection.
const cfTagRetryCooldown = 6 * time.Hour

// cfTagZone records the zone the suppression was set for — an operator
// swapping in a different zone gets tag purge re-attempted immediately
// without waiting out the cooldown.
var cfTagZone atomic.Value // string

// cfTagSuppressed reports whether tag purge is currently suppressed —
// rejected by the API and still inside the retry cooldown. When the
// cooldown has elapsed the flag is cleared so this call re-probes the plan.
func cfTagSuppressed() bool {
	if !cfTagPurgeUnsupported.Load() {
		return false
	}

	if time.Now().Unix() < cfTagRetryAt.Load() {
		return true
	}

	cfTagPurgeUnsupported.Store(false)

	return false
}

func purgeCloudflare(ctx context.Context, cfg Config, req PurgeRequest) error {
	if cfg.CloudflareZoneID == "" || cfg.CloudflareAPIToken == "" {
		return nil // configured provider but credentials absent — treat as disabled
	}

	var firstErr error

	// URL purge — works on every Cloudflare plan, chunked at the 30-file
	// API limit.
	for i := 0; i < len(req.URLs); i += cfFileChunk {
		end := i + cfFileChunk
		if end > len(req.URLs) {
			end = len(req.URLs)
		}

		if err := cfPurge(ctx, cfg, map[string]any{"files": req.URLs[i:end]}); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Tag purge — requires a Cloudflare Enterprise zone. cfPurgeTags marks
	// the zone tag-incapable on rejection so subsequent purges skip the
	// call for the cooldown window; URL purge above still applies. A zone
	// change resets the suppression immediately, and a plan upgrade is
	// picked up when the cooldown elapses — neither needs a restart.
	if len(req.Tags) > 0 {
		if z := cfTagZone.Load(); z != cfg.CloudflareZoneID {
			cfTagPurgeUnsupported.Store(false)
			cfTagRetryAt.Store(0)
			cfTagZone.Store(cfg.CloudflareZoneID)
		}

		if !cfTagSuppressed() {
			if err := cfPurgeTags(ctx, cfg, req.Tags); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// cfPurgeTags runs the tags purge and flags the zone tag-incapable on an
// API-level rejection (non-Enterprise plans) for cfTagRetryCooldown — a
// transport flake does not suppress future tag purges.
func cfPurgeTags(ctx context.Context, cfg Config, tags []string) error {
	err := cfPurge(ctx, cfg, map[string]any{"tags": tags})
	if errors.Is(err, errCFRejected) {
		cfTagPurgeUnsupported.Store(true)
		cfTagRetryAt.Store(time.Now().Add(cfTagRetryCooldown).Unix())
	}

	return err
}

// cfPurge issues one purge_cache call and checks the success flag — the API
// returns HTTP 200 with success:false for plan/permission failures.
func cfPurge(ctx context.Context, cfg Config, body map[string]any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("cdn: marshal cloudflare purge body: %w", err)
	}

	url := fmt.Sprintf("%s/zones/%s/purge_cache", cfAPIBase, cfg.CloudflareZoneID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cdn: build cloudflare purge request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+cfg.CloudflareAPIToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("cdn: cloudflare purge: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Success bool `json:"success"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&out); err != nil {
		return fmt.Errorf("cdn: cloudflare purge decode (status %d): %w", resp.StatusCode, err)
	}

	if !out.Success {
		msg := "unknown"
		if len(out.Errors) > 0 {
			msg = out.Errors[0].Message
		}

		return fmt.Errorf("%w (status %d): %s", errCFRejected, resp.StatusCode, msg)
	}

	return nil
}
