// Package wiki — service.go orchestrates wiki page browsing, revision
// submission, HITL review, and admin management. Feature-flagged via
// wiki.enabled system_setting (§80).
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
//
//	japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/valkey"
)

// WikiPageDTO is the API response shape for a wiki page (index listing).
type WikiPageDTO struct {
	ID         string `json:"id"`
	SpaceID    string `json:"space_id"`
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	Category   string `json:"category"`
	IsFeatured bool   `json:"is_featured"`
	IsLocked   bool   `json:"is_locked"`
	UpdatedAt  string `json:"updated_at"`
	SpaceSlug  string `json:"space_slug,omitempty"`
	SpaceName  string `json:"space_name,omitempty"`
}

// AdminWikiPageDTO extends WikiPageDTO with the latest revision metadata
// needed by the admin review queue (B1). latest_revision_id gates the
// Approve/Reject actions in the frontend; status + excerpt populate the
// table columns.
type AdminWikiPageDTO struct {
	WikiPageDTO
	LatestRevisionID     string `json:"latest_revision_id,omitempty"`
	LatestRevisionStatus string `json:"status,omitempty"`
	Excerpt              string `json:"excerpt,omitempty"`
}

// WikiPageDetailDTO is the full wiki page with current approved revision.
type WikiPageDetailDTO struct {
	WikiPageDTO
	SpaceSlug       string          `json:"space_slug"`
	SpaceName       string          `json:"space_name"`
	CurrentContent  json.RawMessage `json:"current_content"`
	RevisionSummary string          `json:"revision_summary,omitempty"`
	SubmittedBy     string          `json:"submitted_by,omitempty"`
}

// WikiRevisionDTO is the API response shape for a revision.
type WikiRevisionDTO struct {
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	Content     json.RawMessage `json:"content,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	AiGenerated bool            `json:"ai_generated"`
	SubmittedBy string          `json:"submitted_by,omitempty"`
	ReviewedBy  string          `json:"reviewed_by,omitempty"`
	ReviewedAt  string          `json:"reviewed_at,omitempty"`
	CreatedAt   string          `json:"created_at"`
}

// SubmitRevisionInput is the request body for POST /wiki/:space_slug/:page_slug/revisions.
type SubmitRevisionInput struct {
	Content json.RawMessage `json:"content"`
	Summary string          `json:"summary"`
}

// UpdatePageInput is the request body for PATCH /admin/wiki/:page_id.
type UpdatePageInput struct {
	IsLocked   *bool `json:"is_locked"`
	IsFeatured *bool `json:"is_featured"`
	IsActive   *bool `json:"is_active"`
}

// GeneratePageInput is the request body for POST /admin/wiki/generate.
type GeneratePageInput struct {
	SpaceSlug string `json:"space_slug"`
	PageSlug  string `json:"page_slug"`
	Title     string `json:"title"`
	Category  string `json:"category"`
}

// Service holds the business logic for the Destination Wiki.
type Service struct {
	store    *Store
	settings *config.Settings
	vk       valkeycompat.Cmdable
}

// NewService creates a wiki service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetValkey injects the Valkey client used for the jc:wiki_edit_lock lock
// (B6: soft edit lock preventing concurrent wiki edits, §80 / valkey-keys-catalog).
func (s *Service) SetValkey(vk valkeycompat.Cmdable) { s.vk = vk }

// Enabled reads the wiki.enabled system_setting toggle.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "wiki.enabled", false)
}

// CommunityEditsAllowed checks if community edits are allowed (vs mod-only).
func (s *Service) CommunityEditsAllowed(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return true
	}
	return s.settings.GetBoolOr(ctx, tenantID, "wiki.community_edits", true)
}

// MinReputationToEdit returns the reputation threshold for wiki edits.
func (s *Service) MinReputationToEdit(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return 100
	}
	return s.settings.GetIntOr(ctx, tenantID, "wiki.min_reputation_to_edit", 100)
}

// GetWikiIndex returns all active wiki pages for a space (by space slug).
func (s *Service) GetWikiIndex(ctx context.Context, tenantID pgtype.UUID, spaceSlug string) ([]WikiPageDTO, error) {
	pages, err := s.store.ListWikiPagesBySpace(ctx, tenantID, spaceSlug)
	if err != nil {
		return nil, fmt.Errorf("wiki: index: %w", err)
	}
	dtos := make([]WikiPageDTO, 0, len(pages))
	for _, p := range pages {
		dtos = append(dtos, pageToDTO(p))
	}
	return dtos, nil
}

// GetWikiIndexByDestSpaceID returns all active wiki pages for a space by its ID.
// Used by the ?dest= geo-aware query parameter.
func (s *Service) GetWikiIndexByDestSpaceID(ctx context.Context, tenantID, spaceID pgtype.UUID) ([]WikiPageDTO, error) {
	pages, err := s.store.ListWikiPagesBySpaceID(ctx, tenantID, spaceID)
	if err != nil {
		return nil, fmt.Errorf("wiki: index by dest: %w", err)
	}
	dtos := make([]WikiPageDTO, 0, len(pages))
	for _, p := range pages {
		dtos = append(dtos, pageToDTO(p))
	}
	return dtos, nil
}

// GetWikiPage returns a single wiki page with its current approved revision.
func (s *Service) GetWikiPage(ctx context.Context, tenantID pgtype.UUID, spaceSlug, pageSlug string) (WikiPageDetailDTO, error) {
	page, err := s.store.GetWikiPageBySlug(ctx, tenantID, spaceSlug, pageSlug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return WikiPageDetailDTO{}, pgx.ErrNoRows
		}
		return WikiPageDetailDTO{}, fmt.Errorf("wiki: get page: %w", err)
	}

	dto := WikiPageDetailDTO{
		WikiPageDTO: pageToDTO(page.WikiPage),
		SpaceSlug:   page.SpaceSlug,
		SpaceName:   page.SpaceName,
	}

	// Fetch the current approved revision.
	rev, err := s.store.GetApprovedRevision(ctx, page.ID, tenantID)
	if err == nil {
		dto.CurrentContent = json.RawMessage(rev.Content)
		if rev.Summary != nil {
			dto.RevisionSummary = *rev.Summary
		}
		if rev.SubmittedByName != nil {
			dto.SubmittedBy = *rev.SubmittedByName
		}
	}
	// If no approved revision exists, current_content is nil — the page
	// is a stub waiting for its first approved content.

	return dto, nil
}

// GetRevisionHistory returns the revision history for a wiki page.
func (s *Service) GetRevisionHistory(ctx context.Context, tenantID pgtype.UUID, spaceSlug, pageSlug string, page, limit int32) ([]WikiRevisionDTO, int64, error) {
	wikiPage, err := s.store.GetWikiPageBySlug(ctx, tenantID, spaceSlug, pageSlug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, 0, pgx.ErrNoRows
		}
		return nil, 0, fmt.Errorf("wiki: history: get page: %w", err)
	}

	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	revs, total, err := s.store.ListRevisions(ctx, wikiPage.ID, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: history: list: %w", err)
	}

	dtos := make([]WikiRevisionDTO, 0, len(revs))
	for _, r := range revs {
		dtos = append(dtos, revisionToDTO(r))
	}
	return dtos, total, nil
}

// SubmitRevision creates a new pending revision for a wiki page. The
// caller must have the wiki.edit permission and meet the reputation
// threshold (checked by the handler before calling this method).
//
// B6: acquires the jc:wiki_edit_lock:{page_id}:{user_id} Valkey lock before
// inserting to prevent the same user from double-submitting concurrently.
// The lock is released on success or failure; a 5-minute TTL auto-expires
// if the process crashes mid-request.
func (s *Service) SubmitRevision(ctx context.Context, tenantID, userID pgtype.UUID, spaceSlug, pageSlug string, input SubmitRevisionInput) (string, error) {
	if len(input.Content) == 0 {
		return "", fmt.Errorf("content is required")
	}

	page, err := s.store.GetWikiPageBySlug(ctx, tenantID, spaceSlug, pageSlug)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", pgx.ErrNoRows
		}
		return "", fmt.Errorf("wiki: submit: get page: %w", err)
	}

	if page.IsLocked {
		return "", ErrPageLocked
	}

	// B6: acquire the per-user-per-page edit lock. SET NX EX 5m.
	lockKey := valkey.MustKey("wiki_edit_lock:%s:%s", page.ID.String(), userID.String())
	if s.vk != nil {
		acquired, lerr := s.vk.SetNX(ctx, lockKey, "1", 5*time.Minute).Result()
		if lerr != nil {
			slog.Warn("wiki: edit lock acquire failed, proceeding", "error", lerr)
			// Fail open — degrade gracefully if Valkey is unreachable.
		} else if !acquired {
			return "", ErrEditInProgress
		}
		defer func() {
			if err := s.vk.Del(ctx, lockKey).Err(); err != nil {
				slog.Warn("wiki: edit lock release failed", "error", err)
			}
		}()
	}

	revID, err := s.store.SubmitRevision(ctx, tenantID, page.ID, userID, input.Content, input.Summary, false, "", "")
	if err != nil {
		return "", fmt.Errorf("wiki: submit: %w", err)
	}
	return revID.String(), nil
}

// ApproveRevision approves a pending revision (moderator+). Returns the
// approved revision's content (TipTap JSON bytes) so the handler can
// fire-and-forget RAG indexing (B8, §69/ADR-069).
func (s *Service) ApproveRevision(ctx context.Context, revID, pageID, tenantID, reviewerID pgtype.UUID, notes string) ([]byte, error) {
	content, err := s.store.ApproveRevision(ctx, revID, pageID, tenantID, reviewerID, notes)
	if err != nil {
		return nil, fmt.Errorf("wiki: approve: %w", err)
	}
	return content, nil
}

// RejectRevision rejects a pending revision (moderator+).
func (s *Service) RejectRevision(ctx context.Context, revID, tenantID, reviewerID pgtype.UUID, notes string) error {
	if err := s.store.RejectRevision(ctx, revID, tenantID, reviewerID, notes); err != nil {
		return fmt.Errorf("wiki: reject: %w", err)
	}
	return nil
}

// UpdatePage updates admin-managed wiki page fields (lock, feature, active).
func (s *Service) UpdatePage(ctx context.Context, pageID, tenantID pgtype.UUID, input UpdatePageInput) error {
	if err := s.store.UpdatePage(ctx, pageID, tenantID, input.IsLocked, input.IsFeatured, input.IsActive); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("wiki: update page: %w", err)
	}
	return nil
}

// AdminListPages returns all wiki pages for admin management (paginated).
// Each row includes the latest revision metadata for the review queue (B1).
func (s *Service) AdminListPages(ctx context.Context, tenantID pgtype.UUID, spaceID pgtype.UUID, category, q string, isLocked *bool, page, limit int32) ([]AdminWikiPageDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	pages, total, err := s.store.AdminListPages(ctx, AdminListPagesFilter{
		TenantID: tenantID,
		SpaceID:  spaceID,
		Category: category,
		IsLocked: isLocked,
		Q:        q,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: admin list: %w", err)
	}

	dtos := make([]AdminWikiPageDTO, 0, len(pages))
	for _, p := range pages {
		dto := AdminWikiPageDTO{
			WikiPageDTO: pageToDTO(p.WikiPage),
		}
		dto.SpaceSlug = p.SpaceSlug
		dto.SpaceName = p.SpaceName
		if p.LatestRevisionID != nil {
			dto.LatestRevisionID = p.LatestRevisionID.String()
		}
		if p.LatestRevisionStatus != nil {
			dto.LatestRevisionStatus = *p.LatestRevisionStatus
		}
		if p.Excerpt != nil {
			dto.Excerpt = *p.Excerpt
		}
		dtos = append(dtos, dto)
	}
	return dtos, total, nil
}

// ErrPageLocked is returned when a wiki page is locked and cannot accept
// community edits.
var ErrPageLocked = fmt.Errorf("wiki page is locked to mod-only edits")

// ErrEditInProgress is returned when the same user already has a concurrent
// wiki edit submission in-flight for the same page (B6: jc:wiki_edit_lock).
var ErrEditInProgress = fmt.Errorf("a wiki edit is already in progress for this page by this user")

// ErrSpaceNotFound is returned when the destination space slug does not exist.
var ErrSpaceNotFound = fmt.Errorf("destination space not found")

// GeneratePage creates a stub wiki page for a space/category combination,
// resolving the space_slug to a space_id. If the page already exists (same
// slug within the space), it returns the existing page id (idempotent). The
// actual AI draft content is added later as a pending ai_generated revision
// via the Content Agent — this method only ensures the page shell exists
// (B3: replaces the former stub handler).
func (s *Service) GeneratePage(ctx context.Context, tenantID, userID pgtype.UUID, input GeneratePageInput) (string, error) {
	spaceID, err := s.store.GetSpaceIDBySlug(ctx, tenantID, input.SpaceSlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrSpaceNotFound
		}
		return "", fmt.Errorf("wiki: generate: resolve space: %w", err)
	}

	category := input.Category
	if category == "" {
		category = "other"
	}

	pageID, err := s.store.CreatePage(ctx, tenantID, spaceID, userID, input.PageSlug, input.Title, category)
	if err != nil {
		if errors.Is(err, ErrPageExists) {
			// Page already exists — return a sentinel so the handler can 200
			// instead of 201. The caller can still enqueue an AI draft for it.
			return "", ErrPageExists
		}
		return "", fmt.Errorf("wiki: generate: create page: %w", err)
	}
	return pageID.String(), nil
}

// pageToDTO converts a WikiPage to a WikiPageDTO.
func pageToDTO(p WikiPage) WikiPageDTO {
	return WikiPageDTO{
		ID:         p.ID.String(),
		SpaceID:    p.SpaceID.String(),
		Slug:       p.Slug,
		Title:      p.Title,
		Category:   p.Category,
		IsFeatured: p.IsFeatured,
		IsLocked:   p.IsLocked,
		UpdatedAt:  formatTs(p.UpdatedAt),
	}
}

// revisionToDTO converts a WikiRevision to a WikiRevisionDTO.
func revisionToDTO(r WikiRevision) WikiRevisionDTO {
	dto := WikiRevisionDTO{
		ID:          r.ID.String(),
		Status:      r.Status,
		AiGenerated: r.AiGenerated,
		CreatedAt:   formatTs(r.CreatedAt),
	}
	if len(r.Content) > 0 {
		dto.Content = json.RawMessage(r.Content)
	}
	if r.Summary != nil {
		dto.Summary = *r.Summary
	}
	if r.SubmittedByName != nil {
		dto.SubmittedBy = *r.SubmittedByName
	}
	if r.ReviewedBy.Valid {
		dto.ReviewedBy = r.ReviewedBy.String()
	}
	dto.ReviewedAt = formatTs(r.ReviewedAt)
	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
