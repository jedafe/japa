package video

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/media"
	"github.com/jedafe/japa/internal/users"
	"github.com/jedafe/japa/internal/worker"
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
	GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool
}

// AIDispatcher is the subset of the AI Gateway the video service needs to
// generate draft summaries (content_generation category, §106 / ADR-117).
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
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
	dispatcher  AIDispatcher
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

// SetDispatcher injects the AI Gateway dispatcher used to generate video
// draft summaries.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

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

// ListSourcesForAdmin returns the video source admin queue with pagination.
func (s *Service) ListSourcesForAdmin(ctx context.Context, tenantID pgtype.UUID, status, sourceType, q, sort, dir string, limit, offset int32) ([]gen.ListVideoSourcesForAdminRow, int32, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	sourceType = strings.TrimSpace(strings.ToLower(sourceType))
	q = strings.TrimSpace(q)
	sort = strings.TrimSpace(strings.ToLower(sort))
	if sort != "title" && sort != "source_type" && sort != "status" && sort != "created_at" {
		sort = "created_at"
	}
	dir = strings.TrimSpace(strings.ToLower(dir))
	if dir != "asc" && dir != "desc" {
		dir = "desc"
	}
	rows, err := s.store.ListSourcesForAdmin(ctx, tenantID, status, sourceType, q, sort, dir, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountSourcesForAdmin(ctx, tenantID, status, sourceType, q)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// GetSourceForAdmin returns a single video source for admin review.
func (s *Service) GetSourceForAdmin(ctx context.Context, tenantID, id pgtype.UUID) (gen.VideoSource, error) {
	return s.store.GetSourceByID(ctx, tenantID, id)
}

// UpdateSourceStatus sets a video source status (active/removed). Used by
// admin removal / takedown flows.
func (s *Service) UpdateSourceStatus(ctx context.Context, tenantID, id pgtype.UUID, status string) (gen.VideoSource, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	if status != "active" && status != "removed" {
		return gen.VideoSource{}, fmt.Errorf("%w: invalid status", errpkg.ErrInvalid)
	}
	return s.store.UpdateVideoSourceStatus(ctx, tenantID, id, status)
}

// GetImportJobForAdmin returns a single video import job by id without user scoping.
func (s *Service) GetImportJobForAdmin(ctx context.Context, tenantID, id pgtype.UUID) (gen.VideoImportJob, error) {
	return s.store.GetImportJobByID(ctx, tenantID, id)
}

// ListImportJobsForAdmin returns the video import job admin queue with pagination.
func (s *Service) ListImportJobsForAdmin(ctx context.Context, tenantID pgtype.UUID, status, sourceType, q, sort, dir string, limit, offset int32) ([]gen.ListVideoImportJobsForAdminRow, int32, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	sourceType = strings.TrimSpace(strings.ToLower(sourceType))
	q = strings.TrimSpace(q)
	sort = strings.TrimSpace(strings.ToLower(sort))
	if sort != "status" && sort != "created_at" {
		sort = "created_at"
	}
	dir = strings.TrimSpace(strings.ToLower(dir))
	if dir != "asc" && dir != "desc" {
		dir = "desc"
	}
	rows, err := s.store.ListImportJobsForAdmin(ctx, tenantID, status, sourceType, q, sort, dir, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.store.CountImportJobsForAdmin(ctx, tenantID, status, sourceType, q)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// RetryImportJob re-enqueues a failed or completed video import job from the admin queue.
func (s *Service) RetryImportJob(ctx context.Context, tenantID, id, userID pgtype.UUID) (gen.VideoImportJob, error) {
	job, err := s.store.GetImportJobByID(ctx, tenantID, id)
	if err != nil {
		return gen.VideoImportJob{}, err
	}

	// Re-queue as the requesting admin user (or the original job owner).
	enqueueUserID := userID
	if !enqueueUserID.Valid {
		enqueueUserID = job.UserID
	}

	_, err = s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
		ID:            id,
		TenantID:      tenantID,
		Status:        "pending",
		TotalItems:    0,
		ImportedCount: 0,
		FailedCount:   0,
		ErrorLog:      []byte("[]"),
		StartedAt:     pgtype.Timestamptz{Valid: false},
		CompletedAt:   pgtype.Timestamptz{Valid: false},
	})
	if err != nil {
		return gen.VideoImportJob{}, fmt.Errorf("resetting import job: %w", err)
	}

	if s.enqueuer != nil {
		payload, err := json.Marshal(worker.BulkImportPayload{
			ImportJobID: id.String(),
			TenantID:    tenantID.String(),
			UserID:      enqueueUserID.String(),
		})
		if err != nil {
			return gen.VideoImportJob{}, fmt.Errorf("encoding bulk import payload: %w", err)
		}
		if err := s.enqueuer.Enqueue(worker.TypeVideoImportBulk, payload); err != nil {
			return gen.VideoImportJob{}, fmt.Errorf("enqueueing import job: %w", err)
		}
	}

	// Reload after reset.
	return s.store.GetImportJobByID(ctx, tenantID, id)
}

// GenerateDraft uses the AI Gateway to generate a title and body from a
// video source's metadata, then stores the result as a contents row in
// 'in_review' status with ai_generated=true (§106, ADR-117 HITL gate).
func (s *Service) GenerateDraft(ctx context.Context, tenantID, userID, videoSourceID pgtype.UUID) (gen.CreateContentRow, error) {
	if !videoSourceID.Valid {
		return gen.CreateContentRow{}, fmt.Errorf("%w: video_source_id is required", errpkg.ErrRequired)
	}

	source, err := s.store.GetSourceByID(ctx, tenantID, videoSourceID)
	if err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("loading video source: %w", err)
	}

	if s.dispatcher == nil {
		return gen.CreateContentRow{}, fmt.Errorf("AI dispatcher not wired")
	}

	prompt := s.buildSummaryPrompt(source)

	// ai_prompt_hash for audit traceability (ADR-088).
	sum := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(sum[:])

	resp, err := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
		Category: "content_generation",
		Payload: map[string]any{
			"prompt":                prompt,
			"tenant_id":             tenantID,
			"max_completion_tokens": 1024,
		},
	})
	if err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("video: AI generation failed: %w", err)
	}

	reply := ai.ExtractReplyText(resp)
	title, bodyText := s.parseGeneratedSummary(reply, source)

	tipTapBody, bodyHTML, err := s.buildDraftBody(bodyText)
	if err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("building draft body: %w", err)
	}

	contentHash := content.ComputeContentHashWithDistinguisher(tipTapBody, source.ID.String())

	// Merge source field_values with AI provenance metadata.
	fieldValues := map[string]any{}
	if len(source.FieldValues) > 0 {
		_ = json.Unmarshal(source.FieldValues, &fieldValues)
	}
	fieldValues["_ai_provenance"] = map[string]any{
		"ai_generated":        true,
		"ai_model":            "video_summary_v1",
		"prompted_by_user_id": userID.String(),
		"source_type":         "video_source",
		"video_source_id":     source.ID.String(),
		"prompt_hash":         promptHash,
		"prompted_at":         time.Now().UTC().Format(time.RFC3339),
	}
	fvJSON, err := json.Marshal(fieldValues)
	if err != nil {
		return gen.CreateContentRow{}, fmt.Errorf("marshaling field_values: %w", err)
	}

	aiModel := "video_summary_v1"
	created, err := s.content.CreateContentWithQueries(ctx, s.store.write, tenantID, content.CreateContentInput{
		AuthorID:        userID,
		ContentType:     content.TypeVideo,
		Title:           title,
		Body:            tipTapBody,
		BodyHtml:        bodyHTML,
		FieldValues:     fvJSON,
		ContentHash:     &contentHash,
		FeaturedImageID: source.ThumbnailMediaID,
		VideoSourceID:   source.ID,
		Status:          content.StatusInReview,
		Visibility:      content.VisibilityPublic,
		AIGenerated:     true,
		AIGeneratedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
		AIModel:         &aiModel,
	})
	if err != nil {
		if errors.Is(err, content.ErrDuplicateContent) {
			return gen.CreateContentRow{}, fmt.Errorf("%w: an AI draft already exists for this video source", errpkg.ErrAlreadyExists)
		}
		return gen.CreateContentRow{}, fmt.Errorf("creating video draft content: %w", err)
	}

	slog.InfoContext(ctx, "video: AI draft generated",
		"content_id", created.ID.String(),
		"video_source_id", source.ID.String(),
		"tenant_id", tenantID.String(),
		"prompt_hash", promptHash)

	return created, nil
}

// EnqueueGenerateSummary enqueues a video:generate_summary Asynq task for
// background AI draft generation. It is gated by the video.ai_summary.enabled
// system setting (§106, ADR-117).
func (s *Service) EnqueueGenerateSummary(ctx context.Context, tenantID, userID, videoSourceID pgtype.UUID) error {
	if !videoSourceID.Valid {
		return fmt.Errorf("%w: video_source_id is required", errpkg.ErrRequired)
	}

	if s.settings != nil && !s.settings.GetBoolOr(ctx, tenantID, "video.ai_summary.enabled", false) {
		return fmt.Errorf("%w: video AI summary is disabled", errpkg.ErrFeatureDisabled)
	}

	if s.enqueuer == nil {
		return nil
	}

	payload, err := json.Marshal(worker.GenerateSummaryPayload{
		TenantID:      tenantID.String(),
		UserID:        userID.String(),
		VideoSourceID: videoSourceID.String(),
	})
	if err != nil {
		return fmt.Errorf("encoding summary payload: %w", err)
	}

	return s.enqueuer.Enqueue(worker.TypeVideoGenerateSummary, payload)
}

// buildSummaryPrompt assembles the prompt sent to the content_generation
// category. It includes the title, description, category, transcript, and
// thumbnail context so the model can draft a relevant title/body pair.
func (s *Service) buildSummaryPrompt(source gen.VideoSource) string {
	var fieldValues map[string]any
	if len(source.FieldValues) > 0 {
		_ = json.Unmarshal(source.FieldValues, &fieldValues)
	}

	var transcript string
	if t, ok := fieldValues["transcript"].(string); ok {
		transcript = t
	}
	const maxTranscript = 4000
	if len(transcript) > maxTranscript {
		transcript = transcript[:maxTranscript] + "..."
	}

	var duration int
	if d, ok := fieldValues["duration_secs"].(float64); ok {
		duration = int(d)
	}

	category := ""
	if source.Category != nil {
		category = *source.Category
	}

	thumbnailURL := ""
	if source.ThumbnailUrl != nil {
		thumbnailURL = *source.ThumbnailUrl
	}

	var b strings.Builder
	b.WriteString("You are a JapaConnect content assistant. Write a draft post to accompany the following video.\n\n")
	fmt.Fprintf(&b, "Source title: %s\n", source.Title)
	fmt.Fprintf(&b, "Description: %s\n", source.Description)
	fmt.Fprintf(&b, "Category: %s\n", category)
	fmt.Fprintf(&b, "Duration: %d seconds\n", duration)
	fmt.Fprintf(&b, "Source URL: %s\n", source.SourceUrl)
	fmt.Fprintf(&b, "Thumbnail URL: %s\n", thumbnailURL)
	if transcript != "" {
		fmt.Fprintf(&b, "Transcript excerpt: %s\n", transcript)
	}
	b.WriteString("\nGenerate a concise, engaging title and a 2-3 paragraph body summary. " +
		"Return a JSON object with \"title\" and \"body\" keys. " +
		"The body should be plain text (no markdown, no HTML, no citations). " +
		"Do not include markdown code fences, chain-of-thought, or extra commentary.")

	return b.String()
}

// parseGeneratedSummary extracts title and body from the AI reply. It first
// tries to parse the reply as a JSON object with title/body keys; if that
// fails it falls back to using the source title and the raw reply as the body.
func (s *Service) parseGeneratedSummary(reply string, source gen.VideoSource) (title, body string) {
	title = source.Title
	if title == "" {
		title = "AI Video Draft"
	}

	body = source.Description
	if body == "" {
		body = source.SourceUrl
	}

	reply = strings.TrimSpace(stripMarkdownCodeBlocks(reply))
	if reply == "" {
		return title, body
	}

	jsonText := extractTrailingJSONObject(reply)
	if jsonText == "" {
		// Some providers return plain text; try the whole reply as the body.
		jsonText = reply
	}

	var parsed struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	}
	if err := json.Unmarshal([]byte(jsonText), &parsed); err == nil {
		if strings.TrimSpace(parsed.Title) != "" {
			title = strings.TrimSpace(parsed.Title)
		}
		if strings.TrimSpace(parsed.Body) != "" {
			body = strings.TrimSpace(parsed.Body)
		}
	} else {
		// No parseable JSON; use the trimmed reply as the generated body.
		body = reply
	}

	return title, body
}

// buildDraftBody converts plain text into a TipTap doc for the contents.body
// column and a matching simple HTML representation for body_html.
func (s *Service) buildDraftBody(bodyText string) (string, string, error) {
	paragraphs := splitParagraphs(bodyText)

	content := make([]any, 0, len(paragraphs))
	htmlParts := make([]string, 0, len(paragraphs))

	for _, p := range paragraphs {
		if p == "" {
			continue
		}
		content = append(content, map[string]any{
			"type": "paragraph",
			"content": []any{
				map[string]any{
					"type": "text",
					"text": p,
				},
			},
		})
		htmlParts = append(htmlParts, fmt.Sprintf("<p>%s</p>", html.EscapeString(p)))
	}

	doc := map[string]any{
		"type":    "doc",
		"content": content,
	}
	tipTap, err := json.Marshal(doc)
	if err != nil {
		return "", "", err
	}

	return string(tipTap), strings.Join(htmlParts, "\n"), nil
}

// splitParagraphs splits plain text on blank lines and normalises inner
// line breaks to spaces so each paragraph fits a TipTap paragraph node.
func splitParagraphs(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	parts := strings.Split(text, "\n\n")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.ReplaceAll(p, "\n", " ")
		parts[i] = p
	}
	return parts
}

// stripMarkdownCodeBlocks removes leading/trailing triple-backtick fences and
// the optional "json" language tag so the inner JSON/plain text can be parsed.
func stripMarkdownCodeBlocks(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}

	// Skip the opening fence and optional language name.
	afterOpen := strings.TrimPrefix(text, "```")
	afterOpen = strings.TrimSpace(afterOpen)
	afterOpen = strings.TrimPrefix(afterOpen, "json")
	afterOpen = strings.TrimPrefix(afterOpen, "JSON")

	end := strings.LastIndex(afterOpen, "```")
	if end == -1 {
		// No closing fence; return the remainder.
		return strings.TrimSpace(afterOpen)
	}

	return strings.TrimSpace(afterOpen[:end])
}

// extractTrailingJSONObject finds the last top-level JSON object in a string.
// It is useful for reasoning models that emit narrative before the final JSON.
func extractTrailingJSONObject(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	end := strings.LastIndex(text, "}")
	if end == -1 {
		return ""
	}

	nest := 0
	start := -1
	for i := end; i >= 0; i-- {
		switch text[i] {
		case '}':
			nest++
		case '{':
			nest--
			if nest == 0 {
				start = i
				i = -1 // break out of the loop
			}
		}
	}

	if start == -1 {
		return ""
	}

	candidate := text[start : end+1]
	var obj map[string]any
	if err := json.Unmarshal([]byte(candidate), &obj); err == nil {
		return candidate
	}

	return ""
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
