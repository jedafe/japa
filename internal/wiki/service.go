// Package wiki — service.go orchestrates wiki page browsing, revision
// submission, HITL review, and admin management. Feature-flagged via
// wiki.enabled system_setting (§80).
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
//      japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// WikiPageDTO is the API response shape for a wiki page (index listing).
type WikiPageDTO struct {
	ID             string `json:"id"`
	SpaceID        string `json:"space_id"`
	Slug           string `json:"slug"`
	Title          string `json:"title"`
	Category       string `json:"category"`
	IsFeatured     bool   `json:"is_featured"`
	IsLocked       bool   `json:"is_locked"`
	UpdatedAt      string `json:"updated_at"`
	SpaceSlug      string `json:"space_slug,omitempty"`
	SpaceName      string `json:"space_name,omitempty"`
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
	ID              string          `json:"id"`
	Status          string          `json:"status"`
	Content         json.RawMessage `json:"content,omitempty"`
	Summary         string          `json:"summary,omitempty"`
	AiGenerated     bool            `json:"ai_generated"`
	SubmittedBy     string          `json:"submitted_by,omitempty"`
	ReviewedBy      string          `json:"reviewed_by,omitempty"`
	ReviewedAt      string          `json:"reviewed_at,omitempty"`
	CreatedAt       string          `json:"created_at"`
}

// SubmitRevisionInput is the request body for POST /wiki/:space_slug/:page_slug/revisions.
type SubmitRevisionInput struct {
	Content  json.RawMessage `json:"content"`
	Summary  string          `json:"summary"`
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
}

// NewService creates a wiki service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

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

	revID, err := s.store.SubmitRevision(ctx, tenantID, page.ID, userID, input.Content, input.Summary, false, "", "")
	if err != nil {
		return "", fmt.Errorf("wiki: submit: %w", err)
	}
	return revID.String(), nil
}

// ApproveRevision approves a pending revision (moderator+).
func (s *Service) ApproveRevision(ctx context.Context, revID, pageID, tenantID, reviewerID pgtype.UUID, notes string) error {
	if err := s.store.ApproveRevision(ctx, revID, pageID, tenantID, reviewerID, notes); err != nil {
		return fmt.Errorf("wiki: approve: %w", err)
	}
	return nil
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
func (s *Service) AdminListPages(ctx context.Context, tenantID pgtype.UUID, spaceID pgtype.UUID, category, q string, isLocked *bool, page, limit int32) ([]WikiPageDTO, int64, error) {
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

	dtos := make([]WikiPageDTO, 0, len(pages))
	for _, p := range pages {
		dto := pageToDTO(p.WikiPage)
		dto.SpaceSlug = p.SpaceSlug
		dto.SpaceName = p.SpaceName
		dtos = append(dtos, dto)
	}
	return dtos, total, nil
}

// ErrPageLocked is returned when a wiki page is locked and cannot accept
// community edits.
var ErrPageLocked = fmt.Errorf("wiki page is locked to mod-only edits")

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
