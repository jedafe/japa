package video

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"

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

	var synced, failed int
	for _, src := range channels {
		if err := s.syncChannel(ctx, src); err != nil {
			failed++
			slog.WarnContext(ctx, "video: channel sync failed", "source_id", src.ID.String(), "error", err)
			s.recordChannelSyncResult(ctx, src.ID, false, err.Error())
			continue
		}
		synced++
		s.recordChannelSyncResult(ctx, src.ID, true, "")
	}

	slog.InfoContext(ctx, "video: channel sync run complete",
		"due", len(channels), "synced", synced, "failed", failed)

	return nil
}

// syncChannel polls one channel source: resolves the feed URL (cached in
// provider_config), fetches the Atom feed, and imports every video ID not
// already present as a video_sources.external_id.
func (s *Service) syncChannel(ctx context.Context, src gen.IntelSource) error {
	client := newSSRFSafeClient()

	feedURL, err := s.channelFeedURL(ctx, client, src)
	if err != nil {
		return err
	}

	videoIDs, err := fetchChannelVideoIDs(ctx, client, feedURL)
	if err != nil {
		return err
	}

	for _, videoID := range videoIDs {
		if err := s.importChannelVideo(ctx, src, videoID); err != nil {
			slog.WarnContext(ctx, "video: channel sync import failed",
				"source_id", src.ID.String(), "video_id", videoID, "error", err)
		}
	}

	return nil
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
func (s *Service) importChannelVideo(ctx context.Context, src gen.IntelSource, videoID string) error {
	if _, err := s.store.GetSourceByExternalID(ctx, SourceYouTube, videoID); err == nil {
		return nil // already imported
	} else if !errors.Is(err, errpkg.ErrNotFound) {
		return err
	}

	metadata, err := s.resolver.Resolve(ctx, "https://www.youtube.com/watch?v="+videoID)
	if err != nil {
		return err
	}

	result, err := s.createSourceFromMetadata(ctx, metadata)
	if err != nil {
		return err
	}

	// Ensure the video links to the channel row we polled — CreateAutoForVideo
	// can match (or create) a different row when the registered channel URL
	// shape differs from oEmbed's author_url.
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

	return nil
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
