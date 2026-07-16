package redirects

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

const redirectCacheKeyPrefix = "jc:redirect:"
const redirectCacheTTL = 1 * time.Hour

// Enqueuer is the interface for enqueuing background tasks.
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

type Service struct {
	store    *Store
	db       *db.DB
	vk       valkeycompat.Cmdable
	enqueuer Enqueuer
}

func NewService(database *db.DB, vkClient valkeycompat.Cmdable) *Service {
	return &Service{
		store:    &Store{read: gen.New(database.Reader), write: gen.New(database.Writer)},
		db:       database,
		vk:       vkClient,
		enqueuer: &noopEnqueuer{},
	}
}

func (s *Service) SetEnqueuer(e Enqueuer) {
	s.enqueuer = e
}

type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error { return nil }

// ── Admin CRUD ──────────────────────────────────────────────────────────

type ListRedirectsResponse struct {
	Items      []gen.Redirect `json:"items"`
	Page       int32          `json:"page"`
	TotalPages int32          `json:"total_pages"`
}

func (s *Service) ListRedirects(ctx context.Context, tenantID pgtype.UUID, isActive *bool, search string, limit, offset int32) (*ListRedirectsResponse, error) {
	redirects, err := s.store.listRedirects(ctx, tenantID, isActive, search, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing redirects: %w", err)
	}
	total, err := s.store.countRedirects(ctx, tenantID, isActive, search)
	if err != nil {
		return nil, fmt.Errorf("counting redirects: %w", err)
	}
	totalPages := total / limit
	if total%limit > 0 {
		totalPages++
	}
	return &ListRedirectsResponse{Items: redirects, Page: offset / limit, TotalPages: totalPages}, nil
}

func (s *Service) GetRedirect(ctx context.Context, id, tenantID pgtype.UUID) (gen.Redirect, error) {
	return s.store.getRedirect(ctx, id, tenantID)
}

func (s *Service) CreateRedirect(ctx context.Context, tenantID pgtype.UUID, sourcePath, targetURL string, httpStatus int16, notes *string) (gen.Redirect, error) {
	if httpStatus != 301 && httpStatus != 302 && httpStatus != 307 {
		return gen.Redirect{}, ErrInvalidHTTPStatus
	}
	if !strings.HasPrefix(sourcePath, "/") {
		return gen.Redirect{}, fmt.Errorf("source_path must start with /")
	}
	r, err := s.store.createRedirect(ctx, gen.CreateRedirectParams{
		TenantID: tenantID, SourcePath: sourcePath, TargetUrl: targetURL,
		HttpStatus: httpStatus, Notes: notes,
	})
	if err != nil {
		return gen.Redirect{}, err
	}
	s.invalidateValkeyCache(ctx, tenantID.String(), sourcePath)
	return r, nil
}

func (s *Service) UpdateRedirect(ctx context.Context, id, tenantID pgtype.UUID, sourcePath, targetURL string, httpStatus int16, isActive bool, notes *string) (gen.Redirect, error) {
	if httpStatus != 301 && httpStatus != 302 && httpStatus != 307 {
		return gen.Redirect{}, ErrInvalidHTTPStatus
	}
	if !strings.HasPrefix(sourcePath, "/") {
		return gen.Redirect{}, fmt.Errorf("source_path must start with /")
	}
	r, err := s.store.updateRedirect(ctx, gen.UpdateRedirectParams{
		ID: id, TenantID: tenantID, SourcePath: sourcePath, TargetUrl: targetURL,
		HttpStatus: httpStatus, IsActive: isActive, Notes: notes,
	})
	if err != nil {
		return gen.Redirect{}, err
	}
	s.invalidateValkeyCache(ctx, tenantID.String(), sourcePath)
	return r, nil
}

func (s *Service) DeleteRedirect(ctx context.Context, id, tenantID pgtype.UUID) error {
	r, err := s.store.getRedirect(ctx, id, tenantID)
	if err != nil {
		return err
	}
	if err := s.store.deleteRedirect(ctx, id, tenantID); err != nil {
		return err
	}
	s.invalidateValkeyCache(ctx, tenantID.String(), r.SourcePath)
	return nil
}

type BulkActionInput struct {
	Action string   `json:"action"`
	Ids    []string `json:"ids"`
}

func (s *Service) BulkAction(ctx context.Context, tenantID pgtype.UUID, input BulkActionInput) error {
	var ids []pgtype.UUID
	for _, idStr := range input.Ids {
		var id pgtype.UUID
		if err := id.Scan(idStr); err != nil {
			return fmt.Errorf("invalid id %s: %w", idStr, err)
		}
		ids = append(ids, id)
	}
	switch input.Action {
	case "activate":
		return s.store.bulkUpdateRedirectStatus(ctx, tenantID, true, ids)
	case "deactivate":
		return s.store.bulkUpdateRedirectStatus(ctx, tenantID, false, ids)
	case "delete":
		return s.store.bulkDeleteRedirects(ctx, tenantID, ids)
	default:
		return fmt.Errorf("invalid action: %s", input.Action)
	}
}

// ── Runtime Resolution ──────────────────────────────────────────────────

// ResolveRedirect checks Valkey cache then DB for a matching redirect rule.
// Returns the target URL and HTTP status code, or empty string and 0 if no match.
func (s *Service) ResolveRedirect(ctx context.Context, tenantID pgtype.UUID, path string) (string, int) {
	if s.vk == nil {
		return s.resolveFromDB(ctx, tenantID, path)
	}
	cacheKey := redirectCacheKeyPrefix + tenantID.String() + ":" + path
	if cached, err := s.vk.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
		var entry struct {
			TargetURL  string `json:"target_url"`
			HTTPStatus int    `json:"http_status"`
			RedirectID string `json:"redirect_id"`
		}
		if json.Unmarshal([]byte(cached), &entry) == nil && entry.TargetURL != "" {
			go s.recordHit(context.Background(), tenantID, entry.RedirectID)
			return entry.TargetURL, entry.HTTPStatus
		}
	}
	return s.resolveFromDB(ctx, tenantID, path)
}

func (s *Service) resolveFromDB(ctx context.Context, tenantID pgtype.UUID, path string) (string, int) {
	r, err := s.store.getRedirectByPath(ctx, tenantID, path)
	if err != nil {
		return "", 0
	}
	entry, _ := json.Marshal(map[string]interface{}{
		"target_url":  r.TargetUrl,
		"http_status": r.HttpStatus,
		"redirect_id": r.ID.String(),
	})
	if s.vk != nil {
		cacheKey := redirectCacheKeyPrefix + tenantID.String() + ":" + path
		s.vk.Set(context.Background(), cacheKey, string(entry), redirectCacheTTL)
	}
	go s.recordHit(context.Background(), tenantID, r.ID.String())
	return r.TargetUrl, int(r.HttpStatus)
}

func (s *Service) recordHit(ctx context.Context, tenantID pgtype.UUID, idStr string) {
	var id pgtype.UUID
	if err := id.Scan(idStr); err != nil {
		return
	}
	if err := s.store.recordRedirectHit(ctx, id, tenantID); err != nil {
		slog.Warn("failed to record redirect hit", "id", idStr, "error", err)
	}
}

func (s *Service) invalidateValkeyCache(ctx context.Context, tenantIDStr, sourcePath string) {
	if s.vk == nil {
		return
	}
	cacheKey := redirectCacheKeyPrefix + tenantIDStr + ":" + sourcePath
	if err := s.vk.Del(ctx, cacheKey).Err(); err != nil {
		slog.Warn("failed to invalidate redirect cache", "key", cacheKey, "error", err)
	}
}
