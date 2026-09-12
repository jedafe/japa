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
	"math"
	"slices"
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
// hot-reloadable configuration like video.import.youtube_api_key (Rule 3).
type settingsReader interface {
	GetStringOr(ctx context.Context, tenantID pgtype.UUID, key, fallback string) string
	GetBoolOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback bool) bool
	GetIntOr(ctx context.Context, tenantID pgtype.UUID, key string, fallback int) int
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

// SourceRegistry links a video source to the canonical intel_sources registry.
type SourceRegistry interface {
	CreateAutoForVideo(ctx context.Context, tenantID, userID pgtype.UUID, title, sourceURL, thumbnailURL *string) (gen.IntelSource, error)
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
	sources     SourceRegistry
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

// SetSourceRegistry links the video service to the canonical source registry.
func (s *Service) SetSourceRegistry(sr SourceRegistry) { s.sources = sr }

// SetDispatcher injects the AI Gateway dispatcher used to generate video
// draft summaries.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// SetYouTubeProvider stores the production YouTube provider so the bulk
// import worker can reuse its HTTP client and SSRF configuration.
func (s *Service) SetYouTubeProvider(p *YouTubeProvider) { s.ytProvider = p }

const (
	claimStatusApproved  = "approved"
	claimStatusRejected  = "rejected"
	claimStatusWithdrawn = "withdrawn"
	claimStatusPending   = "pending"
	sortFieldCreatedAt   = "created_at"
	sortFieldStatus      = "status"
	sortFieldClaimType   = "claim_type"
	sortFieldTitle       = "title"
	sortFieldSourceType  = "source_type"
	sortDirectionAsc     = "asc"
	sortDirectionDesc    = "desc"
)

// IsEnabled returns true when the video feature is enabled for the tenant.
func (s *Service) IsEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, tenantID, "video.enabled", true)
}

// isPlatformAllowed checks the tenant's video.import.allowed_platforms setting.
// An unparseable value is treated as the default allowlist.
func (s *Service) isPlatformAllowed(ctx context.Context, tenantID pgtype.UUID, platform string) bool {
	if s.settings == nil {
		return true
	}

	raw := s.settings.GetStringOr(ctx, tenantID, "video.import.allowed_platforms", "")
	if raw == "" {
		return true
	}

	var allowed []string
	if err := json.Unmarshal([]byte(raw), &allowed); err != nil {
		return true
	}

	return slices.Contains(allowed, platform)
}

// ResolveURL resolves a public video URL, ensures a synthetic channel user,
// stores the thumbnail, and persists the canonical video source.
func (s *Service) ResolveURL(ctx context.Context, tenantID pgtype.UUID, rawURL string) (ResolveResult, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return ResolveResult{}, errpkg.ErrFeatureDisabled
	}

	if s.settings != nil {
		if token := s.settings.GetStringOr(ctx, tenantID, "video.import.instagram_access_token", ""); token != "" {
			ctx = withInstagramToken(ctx, token)
		}
	}

	metadata, err := s.resolver.Resolve(ctx, rawURL)
	if err != nil {
		return ResolveResult{}, err
	}

	if !s.isPlatformAllowed(ctx, tenantID, metadata.SourceType) {
		return ResolveResult{}, ErrUnsupportedPlatform
	}

	return s.createSourceFromMetadata(ctx, tenantID, metadata)
}

// createSourceFromMetadata persists a video source from already-resolved
// metadata. It is shared by ResolveURL and the bulk import processor.
func (s *Service) createSourceFromMetadata(ctx context.Context, tenantID pgtype.UUID, metadata *Metadata) (ResolveResult, error) {
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

	if metadata.DurationSecs > 0 && metadata.DurationSecs <= math.MaxInt32 {
		d := int32(metadata.DurationSecs)
		durationSecs = &d
	}

	var category *string
	if metadata.Category != "" {
		category = &metadata.Category
	}

	description := metadata.Description

	var intelSourceID pgtype.UUID
	if s.sources != nil {
		src, sErr := s.sources.CreateAutoForVideo(ctx, tenantID, authorID, &metadata.Title, &metadata.SourceURL, thumbnailURL)
		if sErr != nil {
			slog.Warn("video: failed to register canonical source", "source_url", metadata.SourceURL, "error", sErr)
		} else {
			intelSourceID = src.ID
		}
	}

	source, err := s.store.CreateOrUpdateSource(ctx, gen.CreateOrUpdateVideoSourceParams{

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
		SourceID:         intelSourceID,
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
	if !s.IsEnabled(ctx, tenantID) {
		return ResolveResult{}, errpkg.ErrFeatureDisabled
	}

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

	var intelSourceID pgtype.UUID
	if s.sources != nil {
		src, sErr := s.sources.CreateAutoForVideo(ctx, tenantID, uploaderID, &title, &sourceURL, nil)
		if sErr != nil {
			slog.Warn("video: failed to register canonical source for upload", "source_url", sourceURL, "error", sErr)
		} else {
			intelSourceID = src.ID
		}
	}

	source, err := s.store.CreateOrUpdateSource(ctx, gen.CreateOrUpdateVideoSourceParams{

		AuthorUserID:     uploaderID,
		SourceType:       SourceUpload,
		SourceUrl:        sourceURL,
		Title:            title,
		Description:      description,
		ThumbnailMediaID: mediaID,
		FieldValues:      []byte("{}"),
		SourceID:         intelSourceID,
	})
	if err != nil {
		return ResolveResult{}, fmt.Errorf("storing video source: %w", err)
	}

	metadata := &Metadata{
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
	if !s.IsEnabled(ctx, tenantID) {
		return nil, errpkg.ErrFeatureDisabled
	}

	return s.store.ListImportJobsByUser(ctx, tenantID, userID, limit, offset)
}

// SubmitClaim creates a channel ownership claim or a takedown request.
// For claim_type='claim' the target username must identify a synthetic user.
// For claim_type='takedown' a video_source_id must identify the offending source;
// its author becomes the video_author_user_id of the claim.
func (s *Service) SubmitClaim(ctx context.Context, tenantID, claimantUserID pgtype.UUID, username, claimType, evidenceURL, notes, videoSourceID string) (gen.VideoChannelClaim, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return gen.VideoChannelClaim{}, errpkg.ErrFeatureDisabled
	}

	claimType, err := validateClaimType(claimType)
	if err != nil {
		return gen.VideoChannelClaim{}, err
	}

	if err := s.validateClaimant(ctx, tenantID, claimantUserID); err != nil {
		return gen.VideoChannelClaim{}, err
	}

	evidence, notesPtr := optionalStrings(evidenceURL, notes)

	videoAuthorUserID, targetSourceID, err := s.resolveClaimTarget(ctx, tenantID, username, claimType, videoSourceID)
	if err != nil {
		return gen.VideoChannelClaim{}, err
	}

	return s.store.CreateChannelClaim(ctx, gen.CreateVideoChannelClaimParams{

		ClaimantUserID:    claimantUserID,
		VideoAuthorUserID: videoAuthorUserID,
		VideoSourceID:     targetSourceID,
		ClaimType:         claimType,
		EvidenceUrl:       evidence,
		Notes:             notesPtr,
		Status:            "pending",
	})
}

// validateClaimType normalizes and validates the claim type.
func validateClaimType(claimType string) (string, error) {
	claimType = strings.TrimSpace(strings.ToLower(claimType))
	if claimType != "claim" && claimType != "takedown" {
		return "", fmt.Errorf("%w: claim_type must be 'claim' or 'takedown'", errpkg.ErrInvalid)
	}

	return claimType, nil
}

// validateClaimant ensures the claimant is a real user.
func (s *Service) validateClaimant(ctx context.Context, tenantID, claimantUserID pgtype.UUID) error {
	claimantIsSynthetic, err := s.store.GetUserSyntheticStatusByID(ctx, tenantID, claimantUserID)
	if err != nil {
		return fmt.Errorf("resolving claimant: %w", err)
	}

	if claimantIsSynthetic {
		return fmt.Errorf("%w: synthetic users cannot file claims", errpkg.ErrForbidden)
	}

	return nil
}

// optionalStrings builds optional string pointers for non-empty inputs.
func optionalStrings(evidenceURL, notes string) (evidence, notesPtr *string) {
	if strings.TrimSpace(evidenceURL) != "" {
		v := strings.TrimSpace(evidenceURL)
		evidence = &v
	}

	if strings.TrimSpace(notes) != "" {
		v := strings.TrimSpace(notes)
		notesPtr = &v
	}

	return evidence, notesPtr
}

// resolveClaimTarget resolves the synthetic channel or offending source for a claim.
func (s *Service) resolveClaimTarget(ctx context.Context, tenantID pgtype.UUID, username, claimType, videoSourceID string) (videoAuthorUserID, targetSourceID pgtype.UUID, err error) {
	switch claimType {
	case "claim":
		target, err := s.store.GetUserForClaimByUsername(ctx, tenantID, username)
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("resolving channel user: %w", err)
		}

		if !target.IsSynthetic {
			return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("%w: channel is not synthetic", errpkg.ErrInvalid)
		}

		openCount, err := s.store.CountOpenClaimsByAuthor(ctx, tenantID, target.ID)
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("checking existing claims: %w", err)
		}

		if openCount > 0 {
			return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("%w: an open claim already exists for this channel", errpkg.ErrAlreadyExists)
		}

		return target.ID, pgtype.UUID{}, nil

	case "takedown":
		if strings.TrimSpace(videoSourceID) == "" {
			return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("%w: video_source_id is required for takedown", errpkg.ErrRequired)
		}

		var sourceID pgtype.UUID
		if err := sourceID.Scan(videoSourceID); err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("%w: invalid video_source_id", errpkg.ErrInvalid)
		}

		source, err := s.store.GetSourceByID(ctx, tenantID, sourceID)
		if err != nil {
			return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("resolving video source: %w", err)
		}

		return source.AuthorUserID, source.ID, nil
	}

	return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("%w: unknown claim type", errpkg.ErrInvalid)
}

// ReviewClaim transitions a pending or rejected claim to approved, rejected,
// or withdrawn. On approval, ownership is transferred (claim) or the source
// is marked removed (takedown).
func (s *Service) ReviewClaim(ctx context.Context, tenantID, reviewerUserID, claimID pgtype.UUID, status, resolutionNote string) (gen.VideoChannelClaim, error) {
	status, err := s.validateReviewStatus(status)
	if err != nil {
		return gen.VideoChannelClaim{}, err
	}

	resolutionNote = strings.TrimSpace(resolutionNote)
	notePtr := strPtrOrNil(resolutionNote)

	claim, err := db.WithTxResult(ctx, s.store.db.Writer, func(q *gen.Queries) (gen.VideoChannelClaim, error) {
		return s.reviewClaimTx(ctx, q, tenantID, reviewerUserID, claimID, status, notePtr)
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

// validateReviewStatus normalizes and validates the requested claim status.
func (s *Service) validateReviewStatus(status string) (string, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	if status != claimStatusApproved && status != claimStatusRejected && status != claimStatusWithdrawn {
		return "", fmt.Errorf("%w: status must be 'approved', 'rejected', or 'withdrawn'", errpkg.ErrInvalid)
	}

	return status, nil
}

// strPtrOrNil returns a pointer to a non-empty string or nil.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

// reviewClaimTx performs the claim review inside the passed queries (transaction).
func (s *Service) reviewClaimTx(ctx context.Context, q *gen.Queries, tenantID, reviewerUserID, claimID pgtype.UUID, status string, notePtr *string) (gen.VideoChannelClaim, error) {
	existing, err := q.GetVideoChannelClaimByID(ctx,
		claimID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.VideoChannelClaim{}, errpkg.ErrNotFound
		}

		return gen.VideoChannelClaim{}, fmt.Errorf("fetching claim for review: %w", err)
	}

	if err := validateClaimStatusTransition(existing.Status, status); err != nil {
		return gen.VideoChannelClaim{}, err
	}

	if status == claimStatusApproved && existing.Status != claimStatusApproved {
		if err := executeApprovedClaimSideEffects(ctx, q, tenantID, existing); err != nil {
			return gen.VideoChannelClaim{}, err
		}
	}

	return q.UpdateVideoChannelClaimStatus(ctx, gen.UpdateVideoChannelClaimStatusParams{
		Status:         status,
		ReviewedBy:     reviewerUserID,
		ResolutionNote: notePtr,
		ID:             claimID,
	})
}

// validateClaimStatusTransition checks whether a status move is legal.
func validateClaimStatusTransition(from, to string) error {
	allowed := false

	switch from {
	case "pending":
		allowed = to == claimStatusApproved || to == claimStatusRejected || to == claimStatusWithdrawn
	case claimStatusRejected:
		allowed = to == claimStatusApproved || to == claimStatusRejected || to == claimStatusWithdrawn
	case claimStatusApproved, claimStatusWithdrawn:
		allowed = from == to
	}

	if !allowed {
		return fmt.Errorf("%w: cannot transition from %s to %s", errpkg.ErrInvalidStatusTransition, from, to)
	}

	return nil
}

// executeApprovedClaimSideEffects runs the ownership transfer or takedown removal.
func executeApprovedClaimSideEffects(ctx context.Context, q *gen.Queries, tenantID pgtype.UUID, existing gen.VideoChannelClaim) error {
	switch existing.ClaimType {
	case "claim":
		if err := q.ReassignVideoSourcesAuthor(ctx, gen.ReassignVideoSourcesAuthorParams{
			NewAuthorUserID: existing.ClaimantUserID,

			OldAuthorUserID: existing.VideoAuthorUserID,
		}); err != nil {
			return fmt.Errorf("reassigning video sources: %w", err)
		}

		return q.UpdateUserSyntheticToReal(ctx,
			existing.VideoAuthorUserID,
		)

	case "takedown":
		if !existing.VideoSourceID.Valid {
			return fmt.Errorf("%w: takedown claim has no video_source_id", errpkg.ErrInvalid)
		}

		_, err := q.UpdateVideoSourceStatus(ctx, gen.UpdateVideoSourceStatusParams{
			Status: "removed",
			ID:     existing.VideoSourceID,
		})
		if err != nil {
			return fmt.Errorf("removing video source: %w", err)
		}
	}

	return nil
}

// ListClaimsByUser lists the current user's submitted claims.
func (s *Service) ListClaimsByUser(ctx context.Context, tenantID, claimantUserID pgtype.UUID, limit, offset int32) ([]gen.VideoChannelClaim, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return nil, errpkg.ErrFeatureDisabled
	}

	return s.store.ListClaimsByClaimant(ctx, tenantID, claimantUserID, limit, offset)
}

// ListClaimsForAdmin returns the admin review queue with pagination.
func (s *Service) ListClaimsForAdmin(ctx context.Context, tenantID pgtype.UUID, status, claimType, q, sort, dir string, limit, offset int32) ([]gen.ListVideoChannelClaimsByTenantRow, int32, error) {
	status = strings.TrimSpace(strings.ToLower(status))
	claimType = strings.TrimSpace(strings.ToLower(claimType))
	q = strings.TrimSpace(q)

	sort = strings.TrimSpace(strings.ToLower(sort))
	if sort != sortFieldCreatedAt && sort != "status" && sort != "claim_type" {
		sort = sortFieldCreatedAt
	}

	dir = strings.TrimSpace(strings.ToLower(dir))
	if dir != "asc" && dir != sortDirectionDesc {
		dir = sortDirectionDesc
	}

	rows, err := s.store.ListClaimsForAdmin(ctx, tenantID, status, claimType, q, sort, dir, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	total, err := s.store.CountClaimsForAdmin(ctx, tenantID, status, claimType, q)
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
	if sort != "title" && sort != "source_type" && sort != "status" && sort != sortFieldCreatedAt {
		sort = sortFieldCreatedAt
	}

	dir = strings.TrimSpace(strings.ToLower(dir))
	if dir != "asc" && dir != sortDirectionDesc {
		dir = sortDirectionDesc
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
	if sort != "status" && sort != sortFieldCreatedAt {
		sort = sortFieldCreatedAt
	}

	dir = strings.TrimSpace(strings.ToLower(dir))
	if dir != "asc" && dir != sortDirectionDesc {
		dir = sortDirectionDesc
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
		ID: id,

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

			UserID: enqueueUserID.String(),
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
		return gen.CreateContentRow{}, errors.New("AI dispatcher not wired")
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

	if !s.IsEnabled(ctx, tenantID) {
		return errpkg.ErrFeatureDisabled
	}

	if s.settings != nil && !s.settings.GetBoolOr(ctx, tenantID, "video.ai_summary.enabled", false) {
		return fmt.Errorf("%w: video AI summary is disabled", errpkg.ErrFeatureDisabled)
	}

	if s.enqueuer == nil {
		return nil
	}

	payload, err := json.Marshal(worker.GenerateSummaryPayload{

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

func buildEmbedJSON(sourceID pgtype.UUID, metadata *Metadata) json.RawMessage {
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
