package redirects

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/worker"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

const (
	redirectCacheKeyPrefix = "jc:redirect:"
	redirectCacheTTL       = 1 * time.Hour
)

// Enqueuer is the interface for enqueuing background tasks.
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service is an exported type.
type Service struct {
	store    *Store
	db       *db.DB
	vk       valkeycompat.Cmdable
	enqueuer Enqueuer
}

// NewService is an exported function.
func NewService(database *db.DB, vkClient valkeycompat.Cmdable) *Service {
	return &Service{
		store:    &Store{read: gen.New(database.Reader), write: gen.New(database.Writer)},
		db:       database,
		vk:       vkClient,
		enqueuer: &noopEnqueuer{},
	}
}

// SetEnqueuer is an exported method.
func (s *Service) SetEnqueuer(e Enqueuer) {
	s.enqueuer = e
}

// validateTargetURL validates a redirect target_url per ADR-056.
// // Allowed forms:
// - Absolute http/https URLs (e.g., "https://example.com/path")
// - Root-relative paths (e.g., "/new-path")
// // Rejected:
// - Protocol-relative URLs ("//evil.com")
// - Schemes other than http/https (e.g., "javascript:", "data:", "file:")
// - Relative paths without a leading slash ("new-path")
// - Malformed URLs
// // This prevents open-redirect attacks where a malicious target_url could
// redirect users to attacker-controlled domains or trigger dangerous
// schemes via the redirect endpoint.
func validateTargetURL(targetURL string) error {
	if targetURL == "" {
		return ErrInvalidTargetURL
	}

	// Root-relative paths are safe (stay on the same origin).
	if strings.HasPrefix(targetURL, "/") {
		// Reject protocol-relative URLs ("//evil.com").
		if strings.HasPrefix(targetURL, "//") {
			return ErrInvalidTargetURL
		}

		return nil
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return ErrInvalidTargetURL
	}

	// Must be an absolute URL with an http or https scheme.
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrInvalidTargetURL
	}

	if parsed.Host == "" {
		return ErrInvalidTargetURL
	}

	return nil
}

type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error { return nil }

// ── Admin CRUD ──────────────────────────────────────────────────────────

// ListRedirectsResponse is an exported type.
type ListRedirectsResponse struct {
	Items      []gen.Redirect `json:"items"`
	Total      int32          `json:"total"`
	Page       int32          `json:"page"`
	TotalPages int32          `json:"total_pages"`
}

// ListRedirects is an exported method.
func (s *Service) ListRedirects(ctx context.Context, isActive *bool, search, sort, dir string, limit, offset int32) (*ListRedirectsResponse, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}

	redirects, err := s.store.listRedirects(ctx, isActive, search, sort, dir, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing redirects: %w", err)
	}

	total, err := s.store.countRedirects(ctx, isActive, search)
	if err != nil {
		return nil, fmt.Errorf("counting redirects: %w", err)
	}

	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}

	return &ListRedirectsResponse{Items: redirects, Total: total, Page: offset / limit, TotalPages: totalPages}, nil
}

// GetRedirect is an exported method.
func (s *Service) GetRedirect(ctx context.Context, id pgtype.UUID) (gen.Redirect, error) {
	return s.store.getRedirect(ctx, id)
}

// CreateRedirect is an exported method.
func (s *Service) CreateRedirect(ctx context.Context, sourcePath, targetURL string, httpStatus int16, notes *string) (gen.Redirect, error) {
	if httpStatus != 301 && httpStatus != 302 && httpStatus != 307 {
		return gen.Redirect{}, ErrInvalidHTTPStatus
	}

	if !strings.HasPrefix(sourcePath, "/") {
		return gen.Redirect{}, ErrInvalidSourcePath
	}

	if err := validateTargetURL(targetURL); err != nil {
		return gen.Redirect{}, err
	}

	r, err := s.store.createRedirect(ctx, gen.CreateRedirectParams{
		SourcePath: sourcePath, TargetUrl: targetURL,
		HttpStatus: httpStatus, Notes: notes,
	})
	if err != nil {
		return gen.Redirect{}, err
	}

	s.invalidateValkeyCache(ctx, sourcePath)

	return r, nil
}

// UpdateRedirect is an exported method.
func (s *Service) UpdateRedirect(ctx context.Context, id pgtype.UUID, sourcePath, targetURL string, httpStatus int16, isActive *bool, notes *string) (gen.Redirect, error) {
	if httpStatus != 301 && httpStatus != 302 && httpStatus != 307 {
		return gen.Redirect{}, ErrInvalidHTTPStatus
	}

	if !strings.HasPrefix(sourcePath, "/") {
		return gen.Redirect{}, ErrInvalidSourcePath
	}

	if err := validateTargetURL(targetURL); err != nil {
		return gen.Redirect{}, err
	}
	// Fetch the existing redirect so we can invalidate the old source_path's
	// Valkey cache entry if the source_path changed (ADR-056 invalidation).
	existing, err := s.store.getRedirect(ctx, id)
	if err != nil {
		return gen.Redirect{}, err
	}
	// Resolve the is_active field: if the caller omitted it (nil), preserve
	// the existing value rather than defaulting to false.
	resolvedIsActive := existing.IsActive
	if isActive != nil {
		resolvedIsActive = *isActive
	}

	r, err := s.store.updateRedirect(ctx, gen.UpdateRedirectParams{
		ID: id, SourcePath: sourcePath, TargetUrl: targetURL,
		HttpStatus: httpStatus, IsActive: resolvedIsActive, Notes: notes,
	})
	if err != nil {
		return gen.Redirect{}, err
	}

	// no tenant
	// Invalidate the new source_path.
	s.invalidateValkeyCache(ctx, sourcePath)
	// Invalidate the old source_path if it changed.
	if existing.SourcePath != sourcePath {
		s.invalidateValkeyCache(ctx, existing.SourcePath)
	}

	return r, nil
}

// DeleteRedirect is an exported method.
func (s *Service) DeleteRedirect(ctx context.Context, id pgtype.UUID) error {
	r, err := s.store.getRedirect(ctx, id)
	if err != nil {
		return err
	}

	if err := s.store.deleteRedirect(ctx, id); err != nil {
		return err
	}

	s.invalidateValkeyCache(ctx, r.SourcePath)

	return nil
}

// BulkActionInput is an exported type.
type BulkActionInput struct {
	Action string   `json:"action"`
	Ids    []string `json:"ids"`
}

// BulkAction is an exported method.
func (s *Service) BulkAction(ctx context.Context, input BulkActionInput) error {
	// Validate the action before any store access so bad input is rejected
	// without requiring a database connection (matches test expectations).
	switch input.Action {
	case "activate", "deactivate", "delete":
	default:
		return fmt.Errorf("invalid action: %s", input.Action)
	}

	if len(input.Ids) == 0 {
		return errors.New("ids are required")
	}

	var ids []pgtype.UUID

	for _, idStr := range input.Ids {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			return fmt.Errorf("invalid id %s: %w", idStr, err)
		}

		ids = append(ids, id)
	}
	// Fetch source_paths before the mutation so we can invalidate the Valkey cache
	// for each affected redirect (ADR-056 invalidation contract).
	existing, err := s.store.getRedirectsByIDs(ctx, ids)
	if err != nil {
		return fmt.Errorf("fetching redirects for bulk action: %w", err)
	}

	// no tenant

	switch input.Action {
	case "activate":
		if err := s.store.bulkUpdateRedirectStatus(ctx, true, ids); err != nil {
			return err
		}
	case "deactivate":
		if err := s.store.bulkUpdateRedirectStatus(ctx, false, ids); err != nil {
			return err
		}
	case "delete":
		if err := s.store.bulkDeleteRedirects(ctx, ids); err != nil {
			return err
		}
	}
	// Invalidate the Valkey cache for every affected source path.
	for _, r := range existing {
		s.invalidateValkeyCache(ctx, r.SourcePath)
	}

	return nil
}

// ── Runtime Resolution ──────────────────────────────────────────────────

// ResolveRedirect checks Valkey cache then DB for a matching redirect rule.
// Returns the target URL and HTTP status code, or empty string and 0 if no match.
// Per ADR-056, paths longer than maxPathLength (2048 chars) are skipped to
// prevent cache key flooding.
func (s *Service) ResolveRedirect(ctx context.Context, path string) (string, int) {
	if len(path) > maxPathLength {
		return "", 0
	}

	if s.vk == nil {
		return s.resolveFromDB(ctx, path)
	}

	cacheKey := redirectCacheKeyPrefix + path
	if cached, err := s.vk.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
		var entry struct {
			TargetURL  string `json:"target_url"`
			HTTPStatus int    `json:"http_status"`
			RedirectID string `json:"redirect_id"`
		}

		if json.Unmarshal([]byte(cached), &entry) == nil && entry.TargetURL != "" {
			// Hit counting moved to an Asynq task — no per-request goroutines.
			s.enqueueRecordHit(entry.RedirectID)
			return entry.TargetURL, entry.HTTPStatus
		}
	}

	return s.resolveFromDB(ctx, path)
}

func (s *Service) resolveFromDB(ctx context.Context, path string) (string, int) {
	r, err := s.store.getRedirectByPath(ctx, path)
	if err != nil {
		// "no rows" is expected — no redirect for this path. Real DB errors
		// (connection failures, query errors) are logged so ops has visibility;
		// the redirect itself must never fail the request.
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("redirect: DB lookup error", "path", path, "error", err)
		}

		return "", 0
	}

	entry, _ := json.Marshal(map[string]interface{}{
		"target_url":  r.TargetUrl,
		"http_status": r.HttpStatus,
		"redirect_id": r.ID.String(),
	})

	if s.vk != nil {
		cacheKey := redirectCacheKeyPrefix + path
		// Propagate the request context instead of a detached Background one.
		if err := s.vk.Set(ctx, cacheKey, string(entry), redirectCacheTTL).Err(); err != nil {
			slog.Warn("failed to cache redirect", "key", cacheKey, "error", err)
		}
	}

	s.enqueueRecordHit(r.ID.String())

	return r.TargetUrl, int(r.HttpStatus)
}

// enqueueRecordHit hands hit counting to the worker (redirect:record_hit, low
// queue). Best-effort: a failed enqueue must never fail the redirect itself.
func (s *Service) enqueueRecordHit(idStr string) {
	payload, err := json.Marshal(map[string]string{
		"redirect_id": idStr,
	})
	if err != nil {
		slog.Warn("failed to marshal redirect hit payload", "id", idStr, "error", err)
		return
	}

	if err := s.enqueuer.Enqueue(worker.TypeRedirectRecordHit, payload); err != nil {
		slog.Warn("failed to enqueue redirect hit", "id", idStr, "error", err)
	}
}

func (s *Service) invalidateValkeyCache(ctx context.Context, sourcePath string) {
	if s.vk == nil {
		return
	}

	cacheKey := redirectCacheKeyPrefix + sourcePath
	if err := s.vk.Del(ctx, cacheKey).Err(); err != nil {
		slog.Warn("failed to invalidate redirect cache", "key", cacheKey, "error", err)
	}
}
