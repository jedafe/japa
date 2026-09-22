package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/intelligence/sources"
)

// HandleIntelSearchPoll is the intel:search_poll task handler
// (intel-channels Phase 3): search channels stay on the channel scheduler
// (ListDueChannels includes them), and HandleIntelFetchChannel redirects
// them here — the same shape as the xhr_sniff redirect. The handler
// resolves the channel + parent source and runs the shared fetch core:
// the Composite dispatches to SearchFetcher, which emits the SERP page as
// an application/x-ndjson raw doc, then detect_changes → curate.
func HandleIntelSearchPoll(ctx context.Context, t *asynq.Task) error {
	defer recoverPanic("TypeIntelSearchPoll")
	slog.Info("task:intel:search_poll")

	if intelSourcesStore == nil || intelFetcher == nil {
		slog.Warn("task:intel:search_poll: dependencies not wired, skipping")
		return nil
	}

	var p intelChannelPayload
	if err := parsePayloadOrSkip(t, &p); err != nil {
		return err
	}
	channelID, err := scanUUID(p.ChannelID)
	if err != nil {
		return fmt.Errorf("task:intel:search_poll: %w: %w", err, asynq.SkipRetry)
	}

	ch, err := intelSourcesStore.GetChannel(ctx, channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:search_poll: channel no longer exists, skipping", "channel_id", p.ChannelID)
			return nil
		}
		return fmt.Errorf("task:intel:search_poll: load channel: %w", err)
	}
	if !ch.Enabled {
		slog.Info("task:intel:search_poll: channel disabled, skipping", "channel_id", p.ChannelID)
		return nil
	}
	if ch.CrawlMethod != string(sources.CrawlMethodSearch) {
		slog.Info("task:intel:search_poll: channel repurposed, skipping", "channel_id", p.ChannelID, "crawl_method", ch.CrawlMethod)
		return nil
	}

	src, err := intelSourcesStore.Get(ctx, ch.SourceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Info("task:intel:search_poll: parent source no longer exists, skipping", "channel_id", p.ChannelID)
			return nil
		}
		return fmt.Errorf("task:intel:search_poll: load parent source: %w", err)
	}
	if !src.Enabled || src.Status != "active" {
		slog.Info("task:intel:search_poll: parent source not active, skipping", "channel_id", p.ChannelID)
		return nil
	}

	return intelFetchChannel(ctx, "task:intel:search_poll", src, ch, intelFetchOpts{})
}
