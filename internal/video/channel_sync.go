package video

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Channel sync (§106 channel-sync amendment): the video:sync_channels
// Asynq task polls each enabled video_channel intel_source's YouTube Atom
// feed and imports new uploads through createSourceFromMetadata — the same
// path used by bulk import, so synced videos are real video_sources +
// contents rows, not intel_records. The intel fetch pipeline is not
// involved; video_channel sources stay crawl_method='manual'.

// channelProviderConfig keys cached on the video_channel source row so the
// channel page is fetched at most once per source.
type channelProviderConfig struct {
	ChannelID string `json:"channel_id,omitempty"`
	FeedURL   string `json:"feed_url,omitempty"`
}

// ProcessChannelSync is the video:sync_channels worker entry point. Cron
// mode (empty payload) iterates all due channels; per-channel failures are
// recorded on the source row and never fail the batch.
func (s *Service) ProcessChannelSync(ctx context.Context) error {
	if !s.IsEnabled(ctx) || !s.channelSyncEnabled(ctx) {
		return nil
	}

	if s.sources == nil {
		return errors.New("video: channel sync requires the source registry")
	}

	intervalMins := s.channelSyncInt(ctx, "video.channel_sync.interval_minutes", 360)
	batchSize := s.channelSyncInt(ctx, "video.channel_sync.batch_size", 50)

	channels, err := s.sources.ListVideoChannelsDue(ctx, intervalMins, batchSize)
	if err != nil {
		return err
	}

	var synced, skipped, failed int

	for _, src := range channels {
		if err := ctx.Err(); err != nil {
			// Bail on deadline — channels already synced this run have their
			// last_crawled_at stamped, so a retry resumes with the remainder.
			return err
		}

		ok, err := s.syncChannel(ctx, src)

		switch {
		case err != nil:
			if ctx.Err() != nil {
				// Interrupted mid-channel — not a channel failure.
				return ctx.Err()
			}

			failed++

			slog.WarnContext(ctx, "video: channel sync failed", "source_id", src.ID.String(), "error", err)

			s.recordChannelSyncResult(ctx, src.ID, false, err.Error())
		case !ok:
			skipped++
		default:
			synced++

			s.recordChannelSyncResult(ctx, src.ID, true, "")
		}
	}

	slog.InfoContext(ctx, "video: channel sync run complete",
		"due", len(channels), "synced", synced, "skipped", skipped, "failed", failed)

	return nil
}

// syncChannel polls one channel source: resolves the feed URL (cached in
// provider_config), fetches the Atom feed, and imports every video ID not
// already present as a video_sources.external_id. Returns false (no error)
// for sources that cannot be synced at all — non-YouTube channels have no
// uploads feed; skipping them avoids accruing consecutive_failures on rows
// that can never succeed.
func (s *Service) syncChannel(ctx context.Context, src gen.IntelSource) (bool, error) {
	var probe channelProviderConfig
	_ = json.Unmarshal(src.ProviderConfig, &probe)

	if probe.FeedURL == "" && !isYouTubeHost(src.SourceUrl) {
		return false, nil
	}

	client := newSSRFSafeClient()

	videoIDs, err := s.channelVideoIDs(ctx, client, src)
	if err != nil {
		return false, err
	}

	var imported, failed int

	for _, videoID := range videoIDs {
		if err := ctx.Err(); err != nil {
			return true, err
		}

		isNew, err := s.importChannelVideo(ctx, src, videoID)

		switch {
		case err != nil:
			failed++

			slog.WarnContext(ctx, "video: channel sync import failed",
				"source_id", src.ID.String(), "video_id", videoID, "error", err)
		case isNew:
			imported++
		}
	}

	slog.InfoContext(ctx, "video: channel synced",
		"source_id", src.ID.String(), "feed_items", len(videoIDs), "imported", imported, "failed", failed)

	return true, nil
}

// channelVideoIDs returns the video IDs to consider for a channel. On the
// first sync (last_crawled_at unset) the YouTube Data API is used when a key
// is configured — its enumeration reaches deeper than the Atom feed's
// ~15-item window, so a freshly registered channel backfills a larger
// backlog. Keyless or API-failing sources fall back to the feed, and every
// later run is feed-only (incremental by design).
func (s *Service) channelVideoIDs(ctx context.Context, client *http.Client, src gen.IntelSource) ([]string, error) {
	if !src.LastCrawledAt.Valid {
		if ids := s.enumerateChannelViaAPI(ctx, src); len(ids) > 0 {
			return ids, nil
		}
	}

	feedURL, err := s.channelFeedURL(ctx, client, src)
	if err != nil {
		return nil, err
	}

	return fetchChannelVideoIDs(ctx, client, feedURL)
}

// enumerateChannelViaAPI lists a channel's uploads through the YouTube Data
// API, bounded by video.import.max_items_per_run. Returns nil when no key is
// configured, enumeration fails, or the channel has no videos — callers fall
// back to the Atom feed.
func (s *Service) enumerateChannelViaAPI(ctx context.Context, src gen.IntelSource) []string {
	if s.ytProvider == nil {
		return nil
	}

	urls, err := s.ytProvider.SearchChannel(ctx, src.SourceUrl)
	if err != nil {
		if !errors.Is(err, ErrYouTubeAPIKeyMissing) {
			slog.WarnContext(ctx, "video: channel sync: data api enumeration failed, using feed",
				"source_id", src.ID.String(), "error", err)
		}

		return nil
	}

	ids := make([]string, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))

	for _, rawURL := range urls {
		u, err := url.Parse(rawURL)
		if err != nil {
			continue
		}

		id := extractYouTubeID(u)
		if id == "" {
			continue
		}

		if _, dup := seen[id]; dup {
			continue
		}

		seen[id] = struct{}{}

		ids = append(ids, id)
	}

	maxItems := int(s.channelSyncInt(ctx, "video.import.max_items_per_run", 50))
	if len(ids) > maxItems {
		ids = ids[:maxItems]
	}

	return ids
}

// channelFeedURL returns the cached Atom feed URL for a source, resolving
// and persisting channel_id/feed_url in provider_config on first sync.
func (s *Service) channelFeedURL(ctx context.Context, client *http.Client, src gen.IntelSource) (string, error) {
	var cfg channelProviderConfig
	_ = json.Unmarshal(src.ProviderConfig, &cfg)

	if cfg.FeedURL != "" {
		return cfg.FeedURL, nil
	}

	if cfg.ChannelID == "" {
		id, err := resolveYouTubeChannelID(ctx, client, src.SourceUrl)
		if err != nil {
			return "", err
		}

		cfg.ChannelID = id
	}

	cfg.FeedURL = youtubeChannelFeedURL(cfg.ChannelID)

	// Merge into the existing provider_config document — keys this sync does
	// not own (e.g. entity_types) must be preserved (see Store.UpdateProviderConfig).
	merged := map[string]any{}
	_ = json.Unmarshal(src.ProviderConfig, &merged)
	merged["channel_id"] = cfg.ChannelID
	merged["feed_url"] = cfg.FeedURL

	raw, err := json.Marshal(merged)
	if err == nil {
		if err := s.sources.UpdateProviderConfig(ctx, gen.UpdateIntelSourceProviderConfigParams{
			ID:             src.ID,
			ProviderConfig: raw,
		}); err != nil {
			// Non-fatal: resolution simply re-runs next sync.
			slog.WarnContext(ctx, "video: failed to cache channel feed url", "source_id", src.ID.String(), "error", err)
		}
	}

	return cfg.FeedURL, nil
}

// importChannelVideo imports one feed video ID when it is not already known.
// Resolution goes through the keyless oEmbed path; persistence through
// createSourceFromMetadata produces the same row shape as bulk import.
// Returns false with nil error when the video already exists.
func (s *Service) importChannelVideo(ctx context.Context, src gen.IntelSource, videoID string) (bool, error) {
	if _, err := s.store.GetSourceByExternalID(ctx, SourceYouTube, videoID); err == nil {
		return false, nil // already imported
	} else if !errors.Is(err, errpkg.ErrNotFound) {
		return false, err
	}

	metadata, err := s.resolver.Resolve(ctx, "https://www.youtube.com/watch?v="+videoID)
	if err != nil {
		return false, err
	}

	// Attribute the video to the channel row we polled rather than whatever
	// URL shape oEmbed's author_url carries — the registered source_url IS
	// the canonical channel row, so CreateAutoForVideo's GetByURL lookup
	// matches it and no duplicate video_channel row is created for the same
	// channel (e.g. registered /channel/UC… vs oEmbed /@handle).
	if src.SourceUrl != "" {
		metadata.ChannelURL = src.SourceUrl
	}

	result, err := s.createSourceFromMetadata(ctx, metadata)
	if err != nil {
		return false, err
	}

	// Belt-and-suspenders: stamp the polled channel row if anything upstream
	// still resolved a different source_id.
	if result.Source.SourceID != src.ID {
		if err := s.store.SetVideoSourceIntelSource(ctx, result.Source.ID, src.ID); err != nil {
			slog.WarnContext(ctx, "video: failed to stamp channel source link",
				"video_source_id", result.Source.ID.String(), "error", err)
		}
	}

	if err := s.createSyncedVideoContent(ctx, result.Source); err != nil {
		// The video_sources row is the source of truth — a content failure
		// leaves the video importable as a post later.
		slog.WarnContext(ctx, "video: synced content creation failed",
			"video_source_id", result.Source.ID.String(), "error", err)
	}

	return true, nil
}

// createSyncedVideoContent creates the contents row for a feed-synced video,
// authored by the channel account itself (the video_sources author).
func (s *Service) createSyncedVideoContent(ctx context.Context, source gen.VideoSource) error {
	if s.content == nil {
		return nil
	}

	return s.createVideoContentForSource(ctx, source.AuthorUserID, s.channelSyncAutoPublish(ctx), source)
}

// recordChannelSyncResult stamps the sync outcome on the source row via the
// standard crawl-status fields so the control panel shows last-sync state.
func (s *Service) recordChannelSyncResult(ctx context.Context, sourceID pgtype.UUID, success bool, message string) {
	arg := gen.UpdateIntelSourceCrawlStatusParams{ID: sourceID, Success: success}
	if !success {
		arg.LastErrorMessage = &message
	}

	if err := s.sources.UpdateCrawlStatus(ctx, arg); err != nil {
		slog.WarnContext(ctx, "video: failed to record channel sync status", "source_id", sourceID.String(), "error", err)
	}
}

func (s *Service) channelSyncEnabled(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "video.channel_sync.enabled", true)
}

func (s *Service) channelSyncAutoPublish(ctx context.Context) bool {
	if s.settings == nil {
		return true
	}

	return s.settings.GetBoolOr(ctx, "video.channel_sync.auto_publish", true)
}

func (s *Service) channelSyncInt(ctx context.Context, key string, fallback int) int32 {
	if s.settings == nil {
		return int32(fallback) // #nosec G115 — fallback constants are small
	}

	v := s.settings.GetIntOr(ctx, key, fallback)
	if v <= 0 || v > math.MaxInt32 {
		return int32(fallback) // #nosec G115 — fallback constants are small
	}

	return int32(v) // #nosec G115 — v is clamped to MaxInt32 above
}
