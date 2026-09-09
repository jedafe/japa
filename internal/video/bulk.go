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
	dbSourceType, err := normalizeBulkSourceType(req.SourceType)
	if err != nil {
		return gen.VideoImportJob{}, err
	}

	sourceURL := strings.TrimSpace(req.SourceURL)
	query := strings.TrimSpace(req.Query)

	switch dbSourceType {
	case DBSourceYouTubeChannel, DBSourceYouTubePlaylist:
		if sourceURL == "" {
			return gen.VideoImportJob{}, fmt.Errorf("%w: source_url is required for %s", ErrMissingBulkInput, req.SourceType)
		}
	case DBSourceYouTubeSearch:
		if query == "" {
			return gen.VideoImportJob{}, fmt.Errorf("%w: query is required for %s", ErrMissingBulkInput, req.SourceType)
		}
	}

	var targetAuthorID pgtype.UUID
	if req.TargetUsername != nil && strings.TrimSpace(*req.TargetUsername) != "" {
		u, err := s.store.GetUserByUsername(ctx, tenantID, strings.TrimSpace(*req.TargetUsername))
		if err != nil {
			return gen.VideoImportJob{}, fmt.Errorf("resolving target username: %w", err)
		}
		targetAuthorID = u.UserID
	}

	errorLog, _ := json.Marshal([]bulkErrorLogEntry{})

	job, err := s.store.CreateImportJob(ctx, gen.CreateVideoImportJobParams{
		TenantID:           tenantID,
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
		AutoPublish:        req.AutoPublish,
	})
	if err != nil {
		return gen.VideoImportJob{}, fmt.Errorf("creating import job: %w", err)
	}

	if s.enqueuer == nil {
		return gen.VideoImportJob{}, errors.New("no task enqueuer configured")
	}

	payload, err := json.Marshal(BulkImportPayload{
		ImportJobID: job.ID.String(),
		TenantID:    tenantID.String(),
		UserID:      userID.String(),
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
	var importJobID, tenantID, userID pgtype.UUID
	if err := importJobID.Scan(payload.ImportJobID); err != nil {
		return fmt.Errorf("invalid import_job_id: %w", err)
	}
	if err := tenantID.Scan(payload.TenantID); err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}
	if err := userID.Scan(payload.UserID); err != nil {
		return fmt.Errorf("invalid user_id: %w", err)
	}

	job, err := s.store.GetImportJobByID(ctx, tenantID, importJobID)
	if err != nil {
		return fmt.Errorf("loading import job: %w", err)
	}

	startedAt := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	job, err = s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
		ID:            job.ID,
		TenantID:      job.TenantID,
		Status:        "running",
		TotalItems:    0,
		ImportedCount: 0,
		FailedCount:   0,
		ErrorLog:      job.ErrorLog,
		StartedAt:     startedAt,
		CompletedAt:   pgtype.Timestamptz{},
	})
	if err != nil {
		return fmt.Errorf("marking job running: %w", err)
	}

	apiKey, err := s.getYouTubeAPIKey(ctx, tenantID)
	if err != nil {
		_ = s.failJob(ctx, job, "no YouTube Data API key configured")
		return fmt.Errorf("video import %s: %w", job.ID.String(), ErrYouTubeAPIKeyMissing)
	}

	provider := s.bulkYouTubeProvider(apiKey)

	urls, err := s.resolveBulkURLs(ctx, provider, job)
	if err != nil {
		_ = s.failJob(ctx, job, err.Error())
		return fmt.Errorf("resolving bulk urls: %w", err)
	}

	if len(urls) > math.MaxInt32 {
		urls = urls[:math.MaxInt32]
	}
	// #nosec G115 — len(urls) is bounded by the configured max_items_per_run
	// (default 50) and the guard above ensures it fits in int32.
	total := int32(len(urls))
	job, err = s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
		ID:            job.ID,
		TenantID:      job.TenantID,
		Status:        "running",
		TotalItems:    total,
		ImportedCount: 0,
		FailedCount:   0,
		ErrorLog:      job.ErrorLog,
		StartedAt:     startedAt,
	})
	if err != nil {
		slog.WarnContext(ctx, "failed to update import total", "job_id", job.ID, "error", err)
	}

	var imported, failed int32
	var errorLog []bulkErrorLogEntry
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
				errorLog = append(errorLog, bulkErrorLogEntry{URL: rawURL, Message: fmt.Sprintf("content: %s", err.Error())})
				slog.WarnContext(ctx, "failed to create content for bulk video", "url", rawURL, "error", err)
			}
		}

		errorLogBytes, _ := json.Marshal(errorLog)
		job, err = s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
			ID:            job.ID,
			TenantID:      job.TenantID,
			Status:        "running",
			TotalItems:    total,
			ImportedCount: imported,
			FailedCount:   failed,
			ErrorLog:      errorLogBytes,
			StartedAt:     startedAt,
		})
		if err != nil {
			slog.WarnContext(ctx, "failed to update import progress", "job_id", job.ID, "error", err)
		}
	}

	status := "completed"
	if failed > 0 {
		if imported == 0 {
			status = "failed"
		} else {
			status = "partial"
		}
	}

	errorLogBytes, _ := json.Marshal(errorLog)
	_, err = s.store.UpdateImportJob(ctx, gen.UpdateVideoImportJobParams{
		ID:            job.ID,
		TenantID:      job.TenantID,
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

// GetImportJob returns a single import job scoped to the user and tenant.
func (s *Service) GetImportJob(ctx context.Context, tenantID, userID, id pgtype.UUID) (gen.VideoImportJob, error) {
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

func (s *Service) getYouTubeAPIKey(ctx context.Context, tenantID pgtype.UUID) (string, error) {
	key := ""
	if s.settings != nil {
		key = s.settings.GetStringOr(ctx, tenantID, "youtube.data_api_key", "")
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
		ID:            job.ID,
		TenantID:      job.TenantID,
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
