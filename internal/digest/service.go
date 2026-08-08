// Package digest — service.go orchestrates digest edition browsing, draft
// editing, publication, and generation triggers. Feature-flagged via
// digest.enabled and digest.email system_settings (§84).
//
// Ref: japaconnect-engineering/sections/84-community-digest.md
//      japaconnect-engineering/adr/ADR-092-community-digest.md
package digest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// DigestEditionDTO is the API response shape for a digest edition (listing).
type DigestEditionDTO struct {
	ID          string `json:"id"`
	SpaceID     string `json:"space_id,omitempty"`
	Cadence     string `json:"cadence"`
	Status      string `json:"status"`
	ScheduledFor string `json:"scheduled_for,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
	EmailSent   bool   `json:"email_sent"`
	GeneratedBy string `json:"generated_by,omitempty"`
	ReviewedBy  string `json:"reviewed_by,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// DigestEditionDetailDTO is the full digest edition with content.
type DigestEditionDetailDTO struct {
	DigestEditionDTO
	Content json.RawMessage `json:"content"`
}

// UpdateEditionInput is the request body for PATCH /admin/digest/:id.
type UpdateEditionInput struct {
	Content json.RawMessage `json:"content"`
}

// Service holds the business logic for the Community Digest.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a digest service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// Enabled reads the digest.enabled system_setting toggle.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "digest.enabled", false)
}

// EmailEnabled reads the digest.email system_setting toggle.
func (s *Service) EmailEnabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "digest.email", false)
}

// GetLatest returns the most recently published edition for a tenant
// (and optionally a space).
func (s *Service) GetLatest(ctx context.Context, tenantID, spaceID pgtype.UUID) (DigestEditionDetailDTO, error) {
	edition, err := s.store.GetLatestPublished(ctx, tenantID, spaceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DigestEditionDetailDTO{}, pgx.ErrNoRows
		}
		return DigestEditionDetailDTO{}, fmt.Errorf("digest: get latest: %w", err)
	}
	return editionToDetailDTO(edition), nil
}

// Get returns a single published edition by id (public browse).
func (s *Service) Get(ctx context.Context, tenantID, id pgtype.UUID) (DigestEditionDetailDTO, error) {
	edition, err := s.store.GetEdition(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DigestEditionDetailDTO{}, pgx.ErrNoRows
		}
		return DigestEditionDetailDTO{}, fmt.Errorf("digest: get: %w", err)
	}
	if edition.Status != "published" {
		return DigestEditionDetailDTO{}, pgx.ErrNoRows
	}
	return editionToDetailDTO(edition), nil
}

// AdminList returns all digest editions for admin management (paginated).
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, page, limit int32) ([]DigestEditionDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	editions, total, err := s.store.ListEditions(ctx, ListEditionsFilter{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("digest: admin list: %w", err)
	}

	dtos := make([]DigestEditionDTO, 0, len(editions))
	for _, e := range editions {
		dtos = append(dtos, editionToDTO(e))
	}
	return dtos, total, nil
}

// AdminGet returns a single edition (any status) for admin management.
func (s *Service) AdminGet(ctx context.Context, tenantID, id pgtype.UUID) (DigestEditionDetailDTO, error) {
	edition, err := s.store.GetEdition(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return DigestEditionDetailDTO{}, pgx.ErrNoRows
		}
		return DigestEditionDetailDTO{}, fmt.Errorf("digest: admin get: %w", err)
	}
	return editionToDetailDTO(edition), nil
}

// AdminUpdate edits the content of a draft edition before publication.
func (s *Service) AdminUpdate(ctx context.Context, tenantID, id pgtype.UUID, content json.RawMessage) error {
	if len(content) == 0 {
		return fmt.Errorf("content is required")
	}
	if err := s.store.UpdateEdition(ctx, id, tenantID, content); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("digest: admin update: %w", err)
	}
	return nil
}

// AdminPublish publishes a draft or scheduled edition.
func (s *Service) AdminPublish(ctx context.Context, tenantID, reviewerID, id pgtype.UUID) error {
	if err := s.store.PublishEdition(ctx, id, tenantID, reviewerID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("digest: admin publish: %w", err)
	}
	return nil
}

// AdminGenerate triggers generation of a new digest edition. For now this
// returns a 202 Accepted sentinel — the actual generation job enqueue goes
// through the existing §20 AI Gateway (ADR-092).
func (s *Service) AdminGenerate(ctx context.Context, tenantID pgtype.UUID) (string, error) {
	// The actual digest generation task enqueue would go here — for now,
	// we return an accepted sentinel indicating the request was accepted.
	return "accepted", nil
}

// editionToDTO converts a DigestEdition to a DigestEditionDTO.
func editionToDTO(e DigestEdition) DigestEditionDTO {
	dto := DigestEditionDTO{
		ID:        e.ID.String(),
		Cadence:   e.Cadence,
		Status:    e.Status,
		EmailSent: e.EmailSent,
		CreatedAt: formatTs(e.CreatedAt),
	}
	if e.SpaceID.Valid {
		dto.SpaceID = e.SpaceID.String()
	}
	dto.ScheduledFor = formatTs(e.ScheduledFor)
	dto.PublishedAt = formatTs(e.PublishedAt)
	if e.GeneratedBy.Valid {
		dto.GeneratedBy = e.GeneratedBy.String()
	}
	if e.ReviewedBy.Valid {
		dto.ReviewedBy = e.ReviewedBy.String()
	}
	return dto
}

// editionToDetailDTO converts a DigestEdition to a DigestEditionDetailDTO.
func editionToDetailDTO(e DigestEdition) DigestEditionDetailDTO {
	dto := DigestEditionDetailDTO{
		DigestEditionDTO: editionToDTO(e),
	}
	if len(e.Content) > 0 {
		dto.Content = json.RawMessage(e.Content)
	}
	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}
