package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// WebSub subscription lifecycle (intel-channels Phase 3, spec §3/§6):
// intel:websub_subscribe/renew POST the subscription form to the channel's
// hub; the hub's verification callback (HandleWebSubVerify) records the
// GRANTED lease and arms the next intel:websub_renew from it, so a hub
// that shortens the requested term never outlives its renewal. A channel
// that is disabled, deleted, or repurposed ends the chain silently. The
// hub-side verify/push endpoints live in the handlers package.

// intelSiteURL is the public base URL (JC_SITE_URL) used to build the
// hub.callback address — the hub must reach back into this deployment.
// Wired via RegisterIntelSiteURL; empty disables websub tasks.
var intelSiteURL string

// RegisterIntelSiteURL wires the deployment's public base URL for WebSub
// callback construction. Called from server.go alongside
// RegisterIntelDependencies.
func RegisterIntelSiteURL(base string) {
	intelSiteURL = strings.TrimRight(strings.TrimSpace(base), "/")
}

// webSubHTTPClient is the safetransport-backed client for hub POSTs —
// hub URLs come from provider_config (operator-set, but still validated:
// http(s) only, SSRF dial + redirect guard).
var webSubHTTPClient = fetcher.NewSafeClient(30*time.Second, 1<<20)

// intelWebSubPayload is shared by websub_subscribe and websub_renew.
type intelWebSubPayload struct {
	Version   string `json:"version"`
	ChannelID string `json:"channel_id"`
}

// HandleIntelWebSubSubscribe performs the initial (or explicit) hub
// subscription for a webhook channel: POST hub.mode=subscribe with the
// callback URL, topic, secret, and lease request. The hub's verification
// callback arms the renewal chain from the granted lease.
// Config-incomplete channels skip cleanly.
func HandleIntelWebSubSubscribe(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelWebSubSubscribe")
	slog.Info("task:intel:websub_subscribe")

	return intelWebSubSubscribe(ctx, t.Payload())
}

// HandleIntelWebSubRenew re-subscribes before the lease expires; the
// renewal POST's verification callback arms the next window — the
// self-perpetuating half of the subscription lifecycle. A
// disabled/deleted/repurposed channel ends the chain silently.
func HandleIntelWebSubRenew(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelWebSubRenew")
	slog.Info("task:intel:websub_renew")

	return intelWebSubSubscribe(ctx, t.Payload())
}

// intelWebSubSubscribe implements both tasks: load + gate the channel,
// POST the subscription form. The hub's async verification callback
// records the granted lease and arms the next renewal.
func intelWebSubSubscribe(ctx context.Context, raw []byte) error {
	if intelSourcesStore == nil {
		slog.Warn("task:intel:websub: dependencies not wired, skipping")
		return nil
	}

	if intelSiteURL == "" {
		slog.Warn("task:intel:websub: site URL not configured, skipping")
		return nil
	}

	var p intelWebSubPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("task:intel:websub: payload: %w", err)
	}

	channelID, err := scanUUID(p.ChannelID)
	if err != nil {
		return fmt.Errorf("task:intel:websub: channel_id: %w", err)
	}

	ch, err := intelSourcesStore.GetChannel(ctx, channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:websub: channel gone, ending chain", "channel_id", p.ChannelID)
			return nil
		}

		return fmt.Errorf("task:intel:websub: load channel: %w", err)
	}

	if !ch.Enabled || ch.CrawlMethod != string(sources.CrawlMethodWebhook) {
		slog.Info("task:intel:websub: channel disabled or repurposed, ending chain", "channel_id", p.ChannelID, "method", ch.CrawlMethod)
		return nil
	}

	var cfg struct {
		Hub          string `json:"hub"`
		Topic        string `json:"topic"`
		Secret       string `json:"secret"`
		LeaseSeconds int64  `json:"lease_seconds"`
	}

	if err := json.Unmarshal(ch.ProviderConfig, &cfg); err != nil {
		return fmt.Errorf("task:intel:websub: provider_config: %w", err)
	}

	if cfg.Hub == "" || cfg.Topic == "" {
		slog.Info("task:intel:websub: channel missing hub/topic, skipping", "channel_id", ch.ID.String())
		return nil
	}

	if u, perr := url.Parse(cfg.Hub); perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		slog.Warn("task:intel:websub: invalid hub URL, skipping", "channel_id", ch.ID.String(), "hub", cfg.Hub)
		return nil
	}

	callback := fmt.Sprintf("%s/api/v1/intel/inbound/websub/%s", intelSiteURL, ch.ID.String())

	leaseSecs := cfg.LeaseSeconds
	if leaseSecs <= 0 {
		leaseSecs = 864000 // 10 days — the common hub default
	}

	form := url.Values{
		"hub.callback":      {callback},
		"hub.mode":          {"subscribe"},
		"hub.topic":         {cfg.Topic},
		"hub.lease_seconds": {strconv.FormatInt(leaseSecs, 10)},
	}
	if cfg.Secret != "" {
		form.Set("hub.secret", cfg.Secret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Hub, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("task:intel:websub: build hub request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := webSubHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("task:intel:websub: hub request: %w", err)
	}

	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("task:intel:websub: hub returned %d", resp.StatusCode)
	}

	slog.Info("task:intel:websub: subscribed", "channel_id", ch.ID.String(), "hub", cfg.Hub, "topic", cfg.Topic, "requested_lease_s", leaseSecs)

	return nil
}
