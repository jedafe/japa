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
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/intelligence/fetcher"
	"github.com/jedafe/japa/internal/intelligence/sources"
)

// WebSub subscription lifecycle (intel-channels Phase 3, spec §3/§6):
// intel:websub_subscribe POSTs the subscription form to the channel's hub
// and chains an intel:websub_renew before the lease lapses; the renew task
// re-subscribes and re-arms itself until the channel is disabled or
// deleted. The hub-side verify/push endpoints live in the handlers package.

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
// callback URL, topic, secret, and lease request — then arms the renewal
// chain. Config-incomplete channels skip cleanly.
func HandleIntelWebSubSubscribe(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelWebSubSubscribe")
	slog.Info("task:intel:websub_subscribe")

	return intelWebSubSubscribe(ctx, t.Payload())
}

// HandleIntelWebSubRenew re-subscribes before the lease expires and
// re-enqueues itself for the next window — the self-perpetuating half of
// the subscription lifecycle. A disabled/deleted/repurposed channel ends
// the chain silently.
func HandleIntelWebSubRenew(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelWebSubRenew")
	slog.Info("task:intel:websub_renew")

	return intelWebSubSubscribe(ctx, t.Payload())
}

// intelWebSubSubscribe implements both tasks: load + gate the channel,
// POST the subscription form, arm the next renewal on success.
func intelWebSubSubscribe(ctx context.Context, raw []byte) error {
	if intelSourcesStore == nil || intelAsynqClient == nil {
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
		"hub.lease_seconds": {fmt.Sprintf("%d", leaseSecs)},
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

	// Arm the renewal before the lease lapses — margin of the smaller of
	// one day or a tenth of the term keeps short leases honest. The delay
	// floors at one minute: a pathological tiny lease (lease ≤ margin)
	// must not enqueue an immediate re-run — that would POST the hub in
	// a tight unthrottled loop.
	margin := leaseSecs / 10
	if max := int64(24 * 3600); margin > max {
		margin = max
	}
	if margin < 60 {
		margin = 60
	}
	delay := time.Duration(leaseSecs-margin) * time.Second
	if delay < time.Minute {
		delay = time.Minute
	}

	payload := marshalPayload(intelWebSubPayload{Version: intelPayloadVersion, ChannelID: ch.ID.String()})
	if payload != nil {
		if _, err := intelAsynqClient.EnqueueContext(ctx, asynq.NewTask(TypeIntelWebSubRenew, payload),
			append(TaskOpts(TypeIntelWebSubRenew), asynq.ProcessIn(delay))...); err != nil {
			slog.Warn("task:intel:websub: renew enqueue failed", "channel_id", ch.ID.String(), "error", err)
		}
	}

	slog.Info("task:intel:websub: subscribed", "channel_id", ch.ID.String(), "hub", cfg.Hub, "topic", cfg.Topic, "renew_in", delay.String())

	return nil
}
