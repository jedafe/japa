package video

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/users"
)

// ResolveResult is the combined payload returned by ResolveURL and
// UploadVideo. EmbedJSON is a TipTap document containing a `video` node,
// emitted as raw JSON (not base64) so clients can insert it directly.
type ResolveResult struct {
	Source    gen.VideoSource `json:"source"`
	EmbedJSON json.RawMessage `json:"embed_json"`
}

// UserManager creates or returns the synthetic user for a video channel.
type UserManager interface {
	EnsureSyntheticUser(ctx context.Context, tenantID pgtype.UUID, input users.SyntheticUserInput) (pgtype.UUID, error)
}

// MediaManager downloads and stores remote thumbnails (URL imports) and
// stores uploaded video bytes (direct uploads). *media.Service satisfies
// this interface.
type MediaManager interface {
	StoreFromURL(ctx context.Context, tenantID, uploaderID pgtype.UUID, sourceURL, filename string) (pgtype.UUID, error)
	// StoreFromBytes persists raw file bytes as a confirmed media item.
	StoreFromBytes(ctx context.Context, tenantID, uploaderID pgtype.UUID, data []byte, filename string, attribution *media.MediaAttribution) (pgtype.UUID, error)
	// GetMedia returns the stored media record so the caller can derive the
	// serve URL from its storage bucket/path.
	GetMedia(ctx context.Context, tenantID, mediaID pgtype.UUID) (gen.Medium, error)
	// MaxUploadBytes returns the tenant's configured upload size cap.
	MaxUploadBytes(ctx context.Context, tenantID pgtype.UUID) int64
}

// Service coordinates video URL resolution, synthetic channel creation,
// thumbnail storage, and source persistence.
type Service struct {
	store       *Store
	resolver    Resolver
	userManager UserManager
	media       MediaManager
}

// NewService returns a video Service.
func NewService(store *Store, userManager UserManager, media MediaManager, resolver Resolver) *Service {
	return &Service{
		store:       store,
		resolver:    resolver,
		userManager: userManager,
		media:       media,
	}
}

// ResolveURL resolves a public video URL, ensures a synthetic channel user,
// stores the thumbnail, and persists the canonical video source.
func (s *Service) ResolveURL(ctx context.Context, tenantID pgtype.UUID, rawURL string) (ResolveResult, error) {
	metadata, err := s.resolver.Resolve(ctx, rawURL)
	if err != nil {
		return ResolveResult{}, err
	}

	channelKey := metadata.ChannelKey()
	email := fmt.Sprintf("synthetic-%s@synthetic.local", channelKey)

	authorID, err := s.userManager.EnsureSyntheticUser(ctx, tenantID, users.SyntheticUserInput{
		SourceType:  metadata.SourceType,
		Username:    channelKey,
		Email:       email,
		DisplayName: metadata.ChannelName,
		Bio:         metadata.AuthorDesc,
		AvatarURL:   metadata.ChannelLogo,
		CoverURL:    metadata.ChannelBanner,
		Location:    metadata.AuthorLocation,
	})
	if err != nil {
		return ResolveResult{}, fmt.Errorf("ensuring synthetic user: %w", err)
	}

	var thumbnailMediaID pgtype.UUID
	if metadata.ThumbnailURL != "" {
		id, err := s.media.StoreFromURL(ctx, tenantID, authorID, metadata.ThumbnailURL, "thumbnail.jpg")
		if err != nil {
			// Thumbnail storage is best effort; keep the source even if the
			// image cannot be ingested.
			slog.WarnContext(ctx, "failed to store video thumbnail", "url", metadata.ThumbnailURL, "error", err)
		} else {
			thumbnailMediaID = id
		}
	}

	fieldValues := []byte("{}")
	if metadata.DurationSecs > 0 {
		fieldValues, _ = json.Marshal(map[string]any{"duration_secs": metadata.DurationSecs})
	}

	var externalID *string
	if metadata.ExternalID != "" {
		externalID = &metadata.ExternalID
	}
	var thumbnailURL *string
	if metadata.ThumbnailURL != "" {
		thumbnailURL = &metadata.ThumbnailURL
	}
	var durationSecs *int32
	if metadata.DurationSecs > 0 {
		d := int32(metadata.DurationSecs)
		durationSecs = &d
	}
	var category *string
	if metadata.Category != "" {
		category = &metadata.Category
	}
	description := metadata.Description

	source, err := s.store.CreateOrUpdateSource(ctx, gen.CreateOrUpdateVideoSourceParams{
		TenantID:         tenantID,
		AuthorUserID:     authorID,
		SourceType:       metadata.SourceType,
		SourceUrl:        metadata.SourceURL,
		ExternalID:       externalID,
		Title:            metadata.Title,
		Description:      description,
		Category:         category,
		ThumbnailUrl:     thumbnailURL,
		ThumbnailMediaID: thumbnailMediaID,
		DurationSecs:     durationSecs,
		FieldValues:      fieldValues,
	})
	if err != nil {
		return ResolveResult{}, fmt.Errorf("storing video source: %w", err)
	}

	embedJSON := buildEmbedJSON(source.ID, metadata)

	return ResolveResult{Source: source, EmbedJSON: embedJSON}, nil
}

// UploadVideo stores an uploaded video file as media, creates the canonical
// video_sources row with source_type='upload', and returns the source plus a
// TipTap `video` node document. Unlike ResolveURL, the author is the real
// (non-synthetic) uploading user.
func (s *Service) UploadVideo(ctx context.Context, tenantID, uploaderID pgtype.UUID, filename string, data []byte, title, description string) (ResolveResult, error) {
	mediaID, err := s.media.StoreFromBytes(ctx, tenantID, uploaderID, data, filename, nil)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("storing uploaded video: %w", err)
	}

	m, err := s.media.GetMedia(ctx, tenantID, mediaID)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("loading uploaded media: %w", err)
	}

	// The media serve endpoint is same-origin (/api/v1 is proxied to the
	// backend), so the stored source_url is the relative serve path. Video
	// files bypass imgproxy — the serve endpoint streams them directly.
	sourceURL := fmt.Sprintf("/api/v1/media/serve/%s/%s", m.StorageBucket, m.StoragePath)

	if title == "" {
		title = filename
	}

	source, err := s.store.CreateOrUpdateSource(ctx, gen.CreateOrUpdateVideoSourceParams{
		TenantID:         tenantID,
		AuthorUserID:     uploaderID,
		SourceType:       SourceUpload,
		SourceUrl:        sourceURL,
		Title:            title,
		Description:      description,
		ThumbnailMediaID: mediaID,
		FieldValues:      []byte("{}"),
	})
	if err != nil {
		return ResolveResult{}, fmt.Errorf("storing video source: %w", err)
	}

	metadata := &VideoMetadata{
		SourceType: SourceUpload,
		SourceURL:  sourceURL,
		Title:      title,
		EmbedHTML:  fmt.Sprintf(`<video controls preload="metadata" src="%s"></video>`, sourceURL),
	}
	if m.DurationSecs.Valid {
		if secs, convErr := m.DurationSecs.Int64Value(); convErr == nil {
			metadata.DurationSecs = int(secs.Int64)
		}
	}

	return ResolveResult{Source: source, EmbedJSON: buildEmbedJSON(source.ID, metadata)}, nil
}

// GetSourceByID returns a single active video source in the tenant.
func (s *Service) GetSourceByID(ctx context.Context, tenantID, id pgtype.UUID) (gen.VideoSource, error) {
	return s.store.GetSourceByID(ctx, tenantID, id)
}

// MaxUploadBytes returns the tenant's configured upload size cap in bytes.
func (s *Service) MaxUploadBytes(ctx context.Context, tenantID pgtype.UUID) int64 {
	return s.media.MaxUploadBytes(ctx, tenantID)
}

// ListChannelVideosByUsername lists active video sources for a channel username.
func (s *Service) ListChannelVideosByUsername(ctx context.Context, tenantID pgtype.UUID, username string, limit, offset int32) ([]gen.VideoSource, error) {
	return s.store.ListSourcesByUsername(ctx, tenantID, username, limit, offset)
}

// CreateImportJob creates a bulk import job.
func (s *Service) CreateImportJob(ctx context.Context, arg gen.CreateVideoImportJobParams) (gen.VideoImportJob, error) {
	return s.store.CreateImportJob(ctx, arg)
}

// ListImportJobsByUser lists a user's bulk import jobs.
func (s *Service) ListImportJobsByUser(ctx context.Context, tenantID, userID pgtype.UUID, limit, offset int32) ([]gen.VideoImportJob, error) {
	return s.store.ListImportJobsByUser(ctx, tenantID, userID, limit, offset)
}

func buildEmbedJSON(sourceID pgtype.UUID, metadata *VideoMetadata) json.RawMessage {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "video",
				"attrs": map[string]any{
					"video_source_id": sourceID.String(),
					"source_type":     metadata.SourceType,
					"source_url":      metadata.SourceURL,
					"external_id":     metadata.ExternalID,
					"title":           metadata.Title,
					"thumbnail_url":   metadata.ThumbnailURL,
					"embed_html":      metadata.EmbedHTML,
					"duration_secs":   metadata.DurationSecs,
				},
			},
		},
	}
	b, _ := json.Marshal(doc)
	return b
}
