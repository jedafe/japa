package cdn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// BunnyCDN purge API (api.bunny.net), authenticated with the AccessKey
// header. URL purges go through POST /purge?url=...; tag purges use
// POST /pullzone/{id}/purgeCache with a CacheTag body — tag purging works
// on every plan and matches the CDN-Tag response header we emit.
// Var (not const) so tests can point the client at a stub server.
var bunnyAPIBase = "https://api.bunny.net"

func purgeBunny(ctx context.Context, cfg Config, req PurgeRequest) error {
	if cfg.BunnyAPIKey == "" {
		return nil // configured provider but credentials absent — treat as disabled
	}

	var firstErr error

	for _, u := range req.URLs {
		if err := bunnyPurgeURL(ctx, cfg, u); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// CacheTag purges need the pull zone ID — surface the misconfiguration
	// rather than silently dropping tags.
	if len(req.Tags) > 0 {
		if cfg.BunnyPullZoneID == "" {
			return errors.New("cdn: bunny tag purge requires integrations.bunnycdn.pull_zone_id")
		}

		for _, tag := range req.Tags {
			if err := bunnyPurgeTag(ctx, cfg, tag); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// bunnyPurgeURL purges one URL (exact match — the path does not end in "/").
func bunnyPurgeURL(ctx context.Context, cfg Config, target string) error {
	u := fmt.Sprintf("%s/purge?url=%s", bunnyAPIBase, url.QueryEscape(target))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return fmt.Errorf("cdn: build bunny purge request: %w", err)
	}

	httpReq.Header.Set("AccessKey", cfg.BunnyAPIKey)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("cdn: bunny url purge: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("cdn: bunny url purge %q: status %d", target, resp.StatusCode)
	}

	return nil
}

// bunnyPurgeTag purges every cached response carrying the given CDN-Tag.
func bunnyPurgeTag(ctx context.Context, cfg Config, tag string) error {
	payload, err := json.Marshal(map[string]string{"CacheTag": tag})
	if err != nil {
		return fmt.Errorf("cdn: marshal bunny purge body: %w", err)
	}

	u := fmt.Sprintf("%s/pullzone/%s/purgeCache", bunnyAPIBase, cfg.BunnyPullZoneID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cdn: build bunny tag purge request: %w", err)
	}

	httpReq.Header.Set("AccessKey", cfg.BunnyAPIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("cdn: bunny tag purge: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("cdn: bunny tag purge %q: status %d", tag, resp.StatusCode)
	}

	return nil
}
