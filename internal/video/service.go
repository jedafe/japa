package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
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

// TaskEnqueuer is the minimal interface needed to enqueue the
// video:import_bulk Asynq task.
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// settingsReader is the subset of the settings service used to read
// hot-reloadable configuration like youtube.data_api_key.
type settingsReader interface {
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string
}

// ContentCreator creates content rows for imported videos.
// *content.Service satisfies this interface.
type ContentCreator interface {
	CreateContentWithQueries(ctx context.Context, q *gen.Queries, tenantID pgtype.UUID, input content.CreateContentInput) (gen.CreateContentRow, error)
}

// Service coordinates video URL resolution, synthetic channel creation,
// thumbnail storage, and source persistence.
type Service struct {
	store       *Store
	resolver    Resolver
	userManager UserManager
	media       MediaManager
	enqueuer    TaskEnqueuer
	settings    settingsReader
	content     ContentCreator
	ytProvider  *YouTubeProvider
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

// SetEnqueuer injects the Asynq task enqueuer used by StartBulkImport.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetSettings injects the hot-reloadable settings reader.
func (s *Service) SetSettings(r settingsReader) { s.settings = r }

// SetContentService injects the content creator used to publish imported
// videos as content_type='video' rows.
func (s *Service) SetContentService(c ContentCreator) { s.content = c }

// SetYouTubeProvider stores the production YouTube provider so the bulk
// import worker can reuse its HTTP client and SSRF configuration.
func (s *Service) SetYouTubeProvider(p *YouTubeProvider) { s.ytProvider = p }

// ResolveURL resolves a public video URL, ensures a synthetic channel user,
// stores the thumbnail, and persists the canonical video source.
func (s *Service) ResolveURL(ctx context.Context, tenantID pgtype.UUID, rawURL string) (ResolveResult, error) {
	metadata, err := s.resolver.Resolve(ctx, rawURL)
	if err != nil {
		return ResolveResult{}, err
	}
	return s.createSourceFromMetadata(ctx, tenantID, metadata)
}

// createSourceFromMetadata persists a video source from already-resolved
// metadata. It is shared by ResolveURL and the bulk import processor.
func (s *Service) createSourceFromMetadata(ctx context.Context, tenantID pgtype.UUID, metadata *VideoMetadata) (ResolveResult, error) {
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

// SubmitClaim creates a channel ownership claim or a takedown request.
// For claim_type='claim' the target username must identify a synthetic user.
// For claim_type='takedown' a video_source_id must identify the offending source;
// its author becomes the video_author_user_id of the claim.
func (s *Service) SubmitClaim(ctx context.Context, tenantID, claimantUserID pgtype.UUID, username, claimType, evidenceURL, notes, videoSourceID string) (gen.VideoChannelClaim, error) {
	claimType = strings.TrimSpace(strings.ToLower(claimType))
	if claimType != "claim" && claimType != "takedown" {
		return gen.VideoChannelClaim{}, fmt.Errorf("%w: claim_type must be 'claim' or 'takedown'", errpkg.ErrInvalid)
	}

	// Claimants must be real users.
	claimantIsSynthetic, err := s.store.GetUserSyntheticStatusByID(ctx, tenantID, claimantUserID)
	if err != nil {
		return gen.VideoChannelClaim{}, fmt.Errorf("resolving claimant: %w", err)
	}
	if claimantIsSynthetic {
		return gen.VideoChannelClaim{}, fmt.Errorf("%w: synthetic users cannot file claims", errpkg.ErrForbidden)
	}

	var evidence, notesPtr *string
	if strings.TrimSpace(evidenceURL) != "" {
		v := strings.TrimSpace(evidenceURL)
		evidence = &v
	}
	if strings.TrimSpace(notes) != "" {
		v := strings.TrimSpace(notes)
		notesPtr = &v
	}

	var videoAuthorUserID, targetSourceID pgtype.UUID

	switch claimType {
	case "claim":
		target, err := s.store.GetUserForClaimByUsername(ctx, tenantID, username)
		if err != nil {
			return gen.VideoChannelClaim{}, fmt.Errorf("resolving channel user: %w", err)
		}
		if !target.IsSynthetic {
			return gen.VideoChannelClaim{}, fmt.Errorf("%w: channel is not synthetic", errpkg.ErrInvalid)
		}

		// One open (pending or approved) ownership claim per channel.
		openCount, err := s.store.CountOpenClaimsByAuthor(ctx, tenantID, target.ID)
		if err != nil {
			return gen.VideoChannelClaim{}, fmt.Errorf("checking existing claims: %w", err)
		}
		if openCount > 0 {
			return gen.VideoChannelClaim{}, fmt.Errorf("%w: an open claim already exists for this channel", errpkg.ErrAlreadyExists)
		}

		videoAuthorUserID = target.ID

	case "takedown":
		if strings.TrimSpace(videoSourceID) == "" {
			return gen.VideoChannelClaim{}, fmt.Errorf("%w: video_source_id is required for takedown", errpkg.ErrRequired)
		}
		var sourceID pgtype.UUID
		if err := sourceID.Scan(videoSourceID); err != nil {
			return gen.VideoChannelClaim{}, fmt.Errorf("%w: invalid video_source_id", errpkg.ErrInvalid)
		}
		source, err := s.store.GetSourceByID(ctx, tenantID, sourceID)
		if err != nil {
			return gen.VideoChannelClaim{}, fmt.Errorf("resolving video source: %w", err)
		}
		videoAuthorUserID = source.AuthorUserID
		targetSourceID = source.ID
	}

	return s.store.CreateChannelClaim(ctx, gen.CreateVideoChannelClaimParams{
		TenantID:          tenantID,
		ClaimantUserID:    claimantUserID,
		VideoAuthorUserID: videoAuthorUserID,
		VideoSourceID:     targetSourceID,
		ClaimType:         claimType,
		EvidenceUrl:       evidence,
		Notes:             notesPtr,
		Status:            "pending",
	})
}

// ReviewClaim transitions a pending or rejected claim to approved, rejected,
// or withdrawn. On approval, ownership is transferred (claim) or the source
// is marked removed (takedown).
func (s *Service) ReviewClaim(ctx context.Context, tenantID, reviewerUserID, claimID pgtype.UUID, status, resolutionNote string) (gen.VideoChannelClaim, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	if status != "approved" && status != "rejected" && status != "withdrawn" {
		return gen.VideoChannelClaim{}, fmt.Errorf("%w: status must be 'approved', 'rejected', or 'withdrawn'", errpkg.ErrInvalid)
	}

	resolutionNote = strings.TrimSpace(resolutionNote)
	var notePtr *string
	if resolutionNote != "" {
		notePtr = &resolutionNote
	}

	claim, err := db.WithTxResult(ctx, s.store.db.Writer, func(q *gen.Queries) (gen.VideoChannelClaim, error) {
		existing, err := q.GetVideoChannelClaimByID(ctx, gen.GetVideoChannelClaimByIDParams{
			ID:       claimID,
			TenantID: tenantID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return gen.VideoChannelClaim{}, errpkg.ErrNotFound
			}
			return gen.VideoChannelClaim{}, fmt.Errorf("fetching claim for review: %w", err)
		}

		// Validate status transitions.
		allowed := false
		switch existing.Status {
		case "pending":
			allowed = status == "approved" || status == "rejected" || status == "withdrawn"
		case "rejected":
			allowed = status == "approved" || status == "rejected" || status == "withdrawn"
		case "approved", "withdrawn":
			allowed = existing.Status == status
		}
		if !allowed {
			return gen.VideoChannelClaim{}, fmt.Errorf("%w: cannot transition from %s to %s", errpkg.ErrInvalidStatusTransition, existing.Status, status)
		}

		// On approval, execute the side effect before stamping the claim.
		if status == "approved" && existing.Status != "approved" {
			switch existing.ClaimType {
			case "claim":
				if err := q.ReassignVideoSourcesAuthor(ctx, gen.ReassignVideoSourcesAuthorParams{
					NewAuthorUserID: existing.ClaimantUserID,
					TenantID:        tenantID,
					OldAuthorUserID: existing.VideoAuthorUserID,
				}); err != nil {
					return gen.VideoChannelClaim{}, fmt.Errorf("reassigning video sources: %w", err)
				}
				if err := q.UpdateUserSyntheticToReal(ctx, gen.UpdateUserSyntheticToRealParams{
					UserID:   existing.VideoAuthorUserID,
					TenantID: tenantID,
				}); err != nil {
					return gen.VideoChannelClaim{}, fmt.Errorf("promoting synthetic user: %w", err)
				}
			case "takedown":
				// The takedown target is a single video source stored on the claim;
				// set its status to 'removed' to hide the embed.
				if !existing.VideoSourceID.Valid {
					return gen.VideoChannelClaim{}, fmt.Errorf("%w: takedown claim has no video_source_id", errpkg.ErrInvalid)
				}
				if _, err := q.UpdateVideoSourceStatus(ctx, gen.UpdateVideoSourceStatusParams{
					Status:   "removed",
					ID:       existing.VideoSourceID,
					TenantID: tenantID,
				}); err != nil {
					return gen.VideoChannelClaim{}, fmt.Errorf("removing video source: %w", err)
				}
			}
		}

		updated, err := q.UpdateVideoChannelClaimStatus(ctx, gen.UpdateVideoChannelClaimStatusParams{
			Status:         status,
			ReviewedBy:     reviewerUserID,
			ResolutionNote: notePtr,
			ID:             claimID,
			TenantID:       tenantID,
		})
		if err != nil {
			return gen.VideoChannelClaim{}, fmt.Errorf("updating claim status: %w", err)
		}
		return updated, nil
	})
	if err != nil {
		return gen.VideoChannelClaim{}, err
	}

	slog.InfoContext(ctx, "video claim reviewed",
		"claim_id", claimID.String(),
		"tenant_id", tenantID.String(),
		"reviewer_id", reviewerUserID.String(),
		"status", status)

	return claim, nil
}

// ListClaimsByUser lists the current user's submitted claims.
func (s *Service) ListClaimsByUser(ctx context.Context, tenantID, claimantUserID pgtype.UUID, limit, offset int32) ([]gen.VideoChannelClaim, error) {
	return s.store.ListClaimsByClaimant(ctx, tenantID, claimantUserID, limit, offset)
}

// ListClaimsForAdmin returns the admin review queue with pagination.
func (s *Service) ListClaimsForAdmin(ctx context.Context, tenantID pgtype.UUID, status string, limit, offset int32) ([]gen.ListVideoChannelClaimsByTenantRow, int32, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	rows, err := s.store.ListClaimsForAdmin(ctx, tenantID, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountClaimsForAdmin(ctx, tenantID, status)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
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
