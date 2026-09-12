package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// Bulk import source types accepted from the client.
const (
	BulkSourceChannel  = "channel"
	BulkSourcePlaylist = "playlist"
	BulkSourceKeyword  = "keyword"
)

// DB bulk import source types.
const (
	DBSourceYouTubeChannel  = "youtube_channel"
	DBSourceYouTubePlaylist = "youtube_playlist"
	DBSourceYouTubeSearch   = "youtube_search"
)

// ErrInvalidBulkSource is returned when the client sends an unsupported
// source_type.
var ErrInvalidBulkSource = errors.New("invalid bulk import source type")

// ErrMissingBulkInput is returned when a required source URL or query is empty.
var ErrMissingBulkInput = errors.New("source url or query is required")

// BulkImportRequest is the client payload for starting a bulk import.
type BulkImportRequest struct {
	SourceType     string  `json:"source_type"`
	SourceURL      string  `json:"source_url"`
	Query          string  `json:"query"`
	TargetUsername *string `json:"target_username"`
	AutoPublish    bool    `json:"auto_publish"`
}

// BulkImportPayload is the worker payload for the video:import_bulk task.
type BulkImportPayload = worker.BulkImportPayload

type bulkErrorLogEntry struct {
	URL     string `json:"url,omitempty"`
	Message string `json:"message"`
}

func normalizeBulkSourceType(sourceType string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(sourceType)) {
	case BulkSourceChannel:
		return DBSourceYouTubeChannel, nil
	case BulkSourcePlaylist:
		return DBSourceYouTubePlaylist, nil
	case BulkSourceKeyword:
		return DBSourceYouTubeSearch, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidBulkSource, sourceType)
	}
}

// StartBulkImport validates the request, creates a pending import job, and
// enqueues the background worker.
func (s *Service) StartBulkImport(ctx context.Context, tenantID, userID pgtype.UUID, req BulkImportRequest) (gen.VideoImportJob, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return gen.VideoImportJob{}, errpkg.ErrFeatureDisabled
	}

	dbSourceType, sourceURL, query, err := validateBulkRequest(req)
	if err != nil {
		return gen.VideoImportJob{}, err
	}

	targetAuthorID, err := s.resolveTargetAuthor(ctx, tenantID, req.TargetUsername)
	if err != nil {
		return gen.VideoImportJob{}, err
	}

	job, err := s.createBulkImportJob(ctx, tenantID, userID, dbSourceType, sourceURL, query, targetAuthorID, req.AutoPublish)
	if err != nil {
		return gen.VideoImportJob{}, err
	}

	return s.enqueueBulkImport(ctx, job, tenantID, userID)
}

// validateBulkRequest normalizes the bulk import request.
func validateBulkRequest(req BulkImportRequest) (dbSourceType, sourceURL, query string, err error) {
	dbSourceType, err = normalizeBulkSourceType(req.SourceType)
	if err != nil {
		return "", "", "", err
	}

	sourceURL = strings.TrimSpace(req.SourceURL)
	query = strings.TrimSpace(req.Query)

	switch dbSourceType {
	case DBSourceYouTubeChannel, DBSourceYouTubePlaylist:
		if sourceURL == "" {
			return "", "", "", fmt.Errorf("%w: source_url is required for %s", ErrMissingBulkInput, req.SourceType)
		}
	case DBSourceYouTubeSearch:
		if query == "" {
			return "", "", "", fmt.Errorf("%w: query is required for %s", ErrMissingBulkInput, req.SourceType)
		}
	}

	return dbSourceType, sourceURL, query, nil
}

// resolveTargetAuthor resolves an optional target username to a real user ID.
func (s *Service) resolveTargetAuthor(ctx context.Context, tenantID pgtype.UUID, targetUsername *string) (pgtype.UUID, error) {
	if targetUsername == nil || strings.TrimSpace(*targetUsername) == "" {
		return pgtype.UUID{}, nil
	}

	u, err := s.store.GetUserByUsername(ctx, tenantID, strings.TrimSpace(*targetUsername))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("resolving target username: %w", err)
	}

	return u.UserID, nil
}

// createBulkImportJob persists a pending import job record.
func (s *Service) createBulkImportJob(ctx context.Context, tenantID, userID pgtype.UUID, dbSourceType, sourceURL, query string, targetAuthorID pgtype.UUID, autoPublish bool) (gen.VideoImportJob, error) {
	errorLog, _ := json.Marshal([]bulkErrorLogEntry{})

	return s.store.CreateImportJob(ctx, gen.CreateVideoImportJobParams{

		UserID:             userID,
		SourceType:         dbSourceType,
		SourceUrl:          stringPtr(sourceURL),
		Query:              stringPtr(query),
		TargetAuthorUserID: targetAuthorID,
		Status:             "pending",
		TotalItems:         0,
		ImportedCount:      0,
		FailedCount:        0,
		ErrorLog:           errorLog,
		AutoPublish:        autoPublish,
	})
}

// enqueueBulkImport serializes and enqueues the bulk import worker payload.
func (s *Service) enqueueBulkImport(_ context.Context, job gen.VideoImportJob, tenantID, userID pgtype.UUID) (gen.VideoImportJob, error) {
	if s.enqueuer == nil {
		return gen.VideoImportJob{}, errors.New("no task enqueuer configured")
	}

	payload, err := json.Marshal(BulkImportPayload{
		ImportJobID: job.ID.String(),

		UserID: userID.String(),
	})
	if err != nil {
		return gen.VideoImportJob{}, fmt.Errorf("encoding import payload: %w", err)
	}

	if err := s.enqueuer.Enqueue("video:import_bulk", payload); err != nil {
		return gen.VideoImportJob{}, fmt.Errorf("enqueuing import task: %w", err)
	}

	return job, nil
}

// ProcessBulkImport is the worker-side entry point. It resolves the list of
// videos, creates a source (and optionally a content row) for each, and
// updates the job with progress and final status.
func (s *Service) ProcessBulkImport(ctx context.Context, payload BulkImportPayload) error {
	importJobID, tenantID, userID, err := parseBulkImportPayload(payload)
	if err != nil {
		return err
	}

	job, err := s.startBulkImportJob(ctx, importJobID, tenantID)
	if err != nil {
		return err
	}

	apiKey, err := s.getYouTubeAPIKey(ctx, tenantID)
	if err != nil {
		_ = s.failJob(ctx, job, "no YouTube Data API key configured")
		return fmt.Errorf("video import %s: %w", job.ID.String(), ErrYouTubeAPIKeyMissing)
	}

	provider := s.bulkYouTubeProvider(apiKey)

	urls, total, err := s.resolveAndCapBulkURLs(ctx, tenantID, provider, job)
	if err != nil {
		_ = s.failJob(ctx, job, err.Error())
		return fmt.Errorf("resolving bulk urls: %w", err)
	}

	startedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	if err := s.upsertImportJobTotal(ctx, job, startedAt, total); err != nil {
		slog.WarnContext(ctx, "failed to update import total", "job_id", job.ID, "error", err)
	}

	imported, failed, errorLog := s.importBulkURLs(ctx, tenantID, userID, job, startedAt, total, urls, provider)

	return s.finalizeBulkImportJob(ctx, job, startedAt, total, imported, failed, errorLog)
}

// parseBulkImportPayload validates and parses the worker payload IDs.
func parseBulkImportPayload(payload BulkImportPayload) (importJobID, tenantID, userID pgtype.UUID, err error) {
	if err := importJobID.Scan(payload.ImportJobID); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("invalid import_job_id: %w", err)
	}

	if err := tenantID.Scan(payload.TenantID); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("invalid tenant_id: %w", err)
	}

	if err := userID.Scan(payload.UserID); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("invalid user_id: %w", err)
	}

	return importJobID, tenantID, userID, nil
}

// startBulkImportJob loads the import job and marks it running.
func (s *Service) startBulkImportJob(ctx context.Context, importJobID, tenantID pgtype.UUID) (gen.VideoImportJob, error) {
	job, err := s.store.GetImportJobByID(ctx, tenantID, importJobID)
	if err != nil {
		return gen.VideoImportJob{}, fmt.Errorf("loading import job: %w", err)
	}

	startedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	return s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
		ID: job.ID,

		Status:        "running",
		TotalItems:    0,
		ImportedCount: 0,
		FailedCount:   0,
		ErrorLog:      job.ErrorLog,
		StartedAt:     startedAt,
		CompletedAt:   pgtype.Timestamptz{},
	})
}

// resolveAndCapBulkURLs resolves the URLs for a bulk import and clamps them to limits.
func (s *Service) resolveAndCapBulkURLs(ctx context.Context, tenantID pgtype.UUID, provider *YouTubeProvider, job gen.VideoImportJob) ([]string, int32, error) {
	urls, err := s.resolveBulkURLs(ctx, provider, job)
	if err != nil {
		return nil, 0, err
	}

	maxItems := s.maxItemsPerRun(ctx, tenantID)
	if len(urls) > maxItems {
		urls = urls[:maxItems]
	}

	if len(urls) > math.MaxInt32 {
		urls = urls[:math.MaxInt32]
	}

	// #nosec G115 — len(urls) is bounded by the configured max_items_per_run
	// (default 50) and the guard above ensures it fits in int32.
	return urls, int32(len(urls)), nil
}

// upsertImportJobTotal stores the resolved total on the running job.
func (s *Service) upsertImportJobTotal(ctx context.Context, job gen.VideoImportJob, startedAt pgtype.Timestamptz, total int32) error {
	_, err := s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
		ID: job.ID,

		Status:        "running",
		TotalItems:    total,
		ImportedCount: 0,
		FailedCount:   0,
		ErrorLog:      job.ErrorLog,
		StartedAt:     startedAt,
	})

	return err
}

// importBulkURLs processes each URL and updates the job with incremental progress.
func (s *Service) importBulkURLs(ctx context.Context, tenantID, userID pgtype.UUID, job gen.VideoImportJob, startedAt pgtype.Timestamptz, total int32, urls []string, provider *YouTubeProvider) (imported, failed int32, errorLog []bulkErrorLogEntry) {
	if len(job.ErrorLog) > 0 {
		_ = json.Unmarshal(job.ErrorLog, &errorLog)
	}

	for _, rawURL := range urls {
		meta, err := provider.Resolve(ctx, rawURL)
		if err != nil {
			failed++

			errorLog = append(errorLog, bulkErrorLogEntry{URL: rawURL, Message: err.Error()})
			slog.WarnContext(ctx, "failed to resolve bulk video", "url", rawURL, "error", err)

			continue
		}

		result, err := s.createSourceFromMetadata(ctx, tenantID, meta)
		if err != nil {
			failed++

			errorLog = append(errorLog, bulkErrorLogEntry{URL: rawURL, Message: err.Error()})
			slog.WarnContext(ctx, "failed to create source for bulk video", "url", rawURL, "error", err)

			continue
		}

		imported++

		if s.content != nil {
			if err := s.createVideoContent(ctx, tenantID, userID, job, result.Source); err != nil {
				errorLog = append(errorLog, bulkErrorLogEntry{URL: rawURL, Message: "content: " + err.Error()})
				slog.WarnContext(ctx, "failed to create content for bulk video", "url", rawURL, "error", err)
			}
		}

		errorLogBytes, _ := json.Marshal(errorLog)

		if _, err := s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
			ID: job.ID,

			Status:        "running",
			TotalItems:    total,
			ImportedCount: imported,
			FailedCount:   failed,
			ErrorLog:      errorLogBytes,
			StartedAt:     startedAt,
		}); err != nil {
			slog.WarnContext(ctx, "failed to update import progress", "job_id", job.ID, "error", err)
		}
	}

	return imported, failed, errorLog
}

// finalizeBulkImportJob marks the job as completed, partial, or failed.
func (s *Service) finalizeBulkImportJob(ctx context.Context, job gen.VideoImportJob, startedAt pgtype.Timestamptz, total, imported, failed int32, errorLog []bulkErrorLogEntry) error {
	status := "completed"

	if failed > 0 {
		if imported == 0 {
			status = "failed"
		} else {
			status = "partial"
		}
	}

	errorLogBytes, _ := json.Marshal(errorLog)

	_, err := s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
		ID: job.ID,

		Status:        status,
		TotalItems:    total,
		ImportedCount: imported,
		FailedCount:   failed,
		ErrorLog:      errorLogBytes,
		StartedAt:     startedAt,
		CompletedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		return fmt.Errorf("finalizing import job: %w", err)
	}

	return nil
}

// BulkImportPreview is the dry-run result for the studio preview step.
type BulkImportPreview struct {
	Count      int            `json:"count"`
	Sample     map[string]any `json:"sample,omitempty"`
	SourceType string         `json:"source_type"`
	SourceURL  string         `json:"source_url,omitempty"`
	Query      string         `json:"query,omitempty"`
}

// PreviewBulkImport resolves the source list for a bulk import request without
// creating a job or enqueueing a worker. It returns the number of videos that
// would be imported (capped by video.import.max_items_per_run) and a sample of
// the first video so the user can confirm before starting.
// Ref: .jeos/sections/106-video-content-and-import.md.
func (s *Service) PreviewBulkImport(ctx context.Context, tenantID, _ pgtype.UUID, req BulkImportRequest) (BulkImportPreview, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return BulkImportPreview{}, errpkg.ErrFeatureDisabled
	}

	dbSourceType, sourceURL, query, err := validateBulkRequest(req)
	if err != nil {
		return BulkImportPreview{}, err
	}

	apiKey, err := s.getYouTubeAPIKey(ctx, tenantID)
	if err != nil {
		return BulkImportPreview{}, err
	}

	provider := s.bulkYouTubeProvider(apiKey)

	tempJob := gen.VideoImportJob{
		SourceType: dbSourceType,
		SourceUrl:  stringPtr(sourceURL),
		Query:      stringPtr(query),
	}

	urls, err := s.resolveBulkURLs(ctx, provider, tempJob)
	if err != nil {
		return BulkImportPreview{}, err
	}

	limit := s.maxItemsPerRun(ctx, tenantID)
	if len(urls) > limit {
		urls = urls[:limit]
	}

	preview := BulkImportPreview{
		Count:      len(urls),
		SourceType: req.SourceType,
		SourceURL:  sourceURL,
		Query:      query,
	}

	if len(urls) > 0 && s.resolver != nil {
		meta, err := s.resolver.Resolve(ctx, urls[0])
		if err == nil && meta != nil {
			preview.Sample = map[string]any{
				"id":            "preview",
				"source_type":   meta.SourceType,
				"source_url":    meta.SourceURL,
				"external_id":   meta.ExternalID,
				"title":         meta.Title,
				"description":   meta.Description,
				"category":      meta.Category,
				"thumbnail_url": meta.ThumbnailURL,
				"duration_secs": meta.DurationSecs,
				"author_name":   meta.AuthorName,
				"author_handle": meta.AuthorHandle,
				"author_avatar": meta.AuthorAvatar,
				"channel_name":  meta.ChannelName,
				"channel_url":   meta.ChannelURL,
				"status":        "active",
			}
		}
	}

	return preview, nil
}

// RetryMyImportJob re-enqueues an existing import job that the user owns and
// that is not currently running or pending.
// Ref: .jeos/sections/106-video-content-and-import.md.
func (s *Service) RetryMyImportJob(ctx context.Context, tenantID, userID, id pgtype.UUID) (gen.VideoImportJob, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return gen.VideoImportJob{}, errpkg.ErrFeatureDisabled
	}

	job, err := s.GetImportJob(ctx, tenantID, userID, id)
	if err != nil {
		return gen.VideoImportJob{}, err
	}

	if job.Status == "pending" || job.Status == "running" || job.Status == "processing" {
		return gen.VideoImportJob{}, fmt.Errorf("%w: cannot retry a job that is %s", errpkg.ErrInvalidStatusTransition, job.Status)
	}

	return s.RetryImportJob(ctx, tenantID, id, userID)
}

// GetImportJob returns a single import job scoped to the user and tenant.
func (s *Service) GetImportJob(ctx context.Context, tenantID, userID, id pgtype.UUID) (gen.VideoImportJob, error) {
	if !s.IsEnabled(ctx, tenantID) {
		return gen.VideoImportJob{}, errpkg.ErrFeatureDisabled
	}

	job, err := s.store.GetImportJobByID(ctx, tenantID, id)
	if err != nil {
		return gen.VideoImportJob{}, err
	}

	if job.UserID != userID {
		return gen.VideoImportJob{}, errpkg.ErrNotFound
	}

	return job, nil
}

func (s *Service) resolveBulkURLs(ctx context.Context, p *YouTubeProvider, job gen.VideoImportJob) ([]string, error) {
	switch job.SourceType {
	case DBSourceYouTubeChannel:
		if job.SourceUrl == nil || *job.SourceUrl == "" {
			return nil, ErrMissingBulkInput
		}

		return p.SearchChannel(ctx, *job.SourceUrl)
	case DBSourceYouTubePlaylist:
		if job.SourceUrl == nil || *job.SourceUrl == "" {
			return nil, ErrMissingBulkInput
		}

		return p.SearchPlaylist(ctx, *job.SourceUrl)
	case DBSourceYouTubeSearch:
		if job.Query == nil || *job.Query == "" {
			return nil, ErrMissingBulkInput
		}

		return p.SearchKeyword(ctx, *job.Query)
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidBulkSource, job.SourceType)
	}
}

func (s *Service) maxItemsPerRun(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return 50
	}

	return s.settings.GetIntOr(ctx, tenantID, "video.import.max_items_per_run", 50)
}

func (s *Service) getYouTubeAPIKey(ctx context.Context, tenantID pgtype.UUID) (string, error) {
	key := ""
	if s.settings != nil {
		// Canonical key is video.import.youtube_api_key (§106). Fallback to the
		// legacy youtube.data_api_key key for compatibility with earlier installs.
		key = s.settings.GetStringOr(ctx, tenantID, "video.import.youtube_api_key", "")
		if key == "" {
			key = s.settings.GetStringOr(ctx, tenantID, "youtube.data_api_key", "")
		}
	}

	if key == "" {
		key = os.Getenv("JC_YOUTUBE_API_KEY")
	}

	if key == "" {
		return "", ErrYouTubeAPIKeyMissing
	}

	return key, nil
}

func (s *Service) bulkYouTubeProvider(apiKey string) *YouTubeProvider {
	if s.ytProvider != nil {
		return s.ytProvider.CloneWithAPIKey(apiKey)
	}

	return NewYouTubeProvider(WithAPIKey(apiKey))
}

func (s *Service) failJob(ctx context.Context, job gen.VideoImportJob, message string) error {
	errorLog, _ := json.Marshal([]bulkErrorLogEntry{{Message: message}})
	_, err := s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
		ID: job.ID,

		Status:        "failed",
		TotalItems:    0,
		ImportedCount: 0,
		FailedCount:   0,
		ErrorLog:      errorLog,
		StartedAt:     pgtype.Timestamptz{Time: time.Now(), Valid: true},
		CompletedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})

	return err
}

func (s *Service) createVideoContent(ctx context.Context, tenantID, userID pgtype.UUID, job gen.VideoImportJob, source gen.VideoSource) error {
	if s.content == nil {
		return nil
	}

	authorID := userID
	if job.TargetAuthorUserID.Valid {
		authorID = job.TargetAuthorUserID
	}

	status := content.StatusDraft
	spaceID := pgtype.UUID{}

	if job.AutoPublish {
		spaceID, _ = s.store.GetDefaultSpaceForUser(ctx, tenantID, authorID)
		if spaceID.Valid {
			status = content.StatusPublished
		} else {
			slog.WarnContext(ctx, "auto_publish enabled but no default space; creating draft content",
				"job_id", job.ID, "tenant_id", tenantID)
		}
	}

	title := source.Title
	if title == "" {
		title = "Untitled"
	}

	body := source.Description
	if body == "" {
		body = source.SourceUrl
	}

	var contentHash *string

	h := content.ComputeContentHashWithDistinguisher(body, source.ID.String())
	contentHash = &h

	_, err := s.content.CreateContentWithQueries(ctx, s.store.write, tenantID, content.CreateContentInput{
		AuthorID:      authorID,
		ContentType:   content.TypeVideo,
		Title:         title,
		Body:          body,
		BodyHtml:      "",
		FieldValues:   []byte("{}"),
		ContentHash:   contentHash,
		SpaceID:       spaceID,
		VideoSourceID: source.ID,
		Status:        status,
		Visibility:    content.VisibilityPublic,
	})

	return err
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
