// Package attestationguide — service.go orchestrates attestation
// pathway lookup, creation, HITL approval, and community update
// moderation. Pathways are curated guides for document
// legalization/apostille across jurisdictions. Feature-flagged via the
// attestation_guide.enabled system_setting (§97, ADR-105).
//
// Ref: japaconnect-engineering/sections/97-attestation-guide.md
//      japaconnect-engineering/adr/ADR-105-attestation-guide.md
package attestationguide

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// PathwayDTO is the API response shape for a single attestation pathway.
type PathwayDTO struct {
	ID                    string          `json:"id"`
	DocumentType          string          `json:"document_type"`
	IssuingSpaceID        string          `json:"issuing_space_id"`
	IssuingSpaceName      string          `json:"issuing_space_name,omitempty"`
	IssuingSpaceCode      string          `json:"issuing_space_code,omitempty"`
	DestinationSpaceID    string          `json:"destination_space_id"`
	DestinationSpaceName  string          `json:"destination_space_name,omitempty"`
	DestinationSpaceCode  string          `json:"destination_space_code,omitempty"`
	ApostilleRequired     bool            `json:"apostille_required"`
	PathwaySteps          json.RawMessage `json:"pathway_steps"`
	CommunityLastVerified string          `json:"community_last_verified,omitempty"`
	Status                string          `json:"status"`
	IsActive              bool            `json:"is_active"`
	AIGenerated           bool            `json:"ai_generated"`
	AIModel               string          `json:"ai_model,omitempty"`
	AIPromptHash          string          `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy        string          `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedAt        string          `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes       string          `json:"hitl_review_notes,omitempty"`
	IsJapabotIndexed      bool            `json:"is_japabot_indexed"`
	CreatedAt             string          `json:"created_at"`
	UpdatedAt             string          `json:"updated_at"`
}

// CommunityUpdateDTO is the API response shape for a community update.
type CommunityUpdateDTO struct {
	ID             string `json:"id"`
	PathwayID      string `json:"pathway_id"`
	UserID         string `json:"user_id"`
	StepIndex      *int32 `json:"step_index,omitempty"`
	FeeEstimate    string `json:"fee_estimate,omitempty"`
	TurnaroundDays *int32 `json:"turnaround_days,omitempty"`
	Note           string `json:"note,omitempty"`
	Status         string `json:"status"`
	ReviewedBy     string `json:"reviewed_by,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// CreatePathwayInput is the request body for POST /admin/attestation.
type CreatePathwayInput struct {
	DocumentType          string          `json:"document_type"`
	IssuingSpaceID        string          `json:"issuing_space_id"`
	DestinationSpaceID    string          `json:"destination_space_id"`
	ApostilleRequired     bool            `json:"apostille_required"`
	PathwaySteps          json.RawMessage `json:"pathway_steps"`
	CommunityLastVerified string          `json:"community_last_verified"`
	Status                string          `json:"status"`
	IsActive              *bool           `json:"is_active"`
	AIModel               string          `json:"ai_model"`
	AIPromptHash          string          `json:"ai_prompt_hash"`
	HITLReviewNotes       string          `json:"hitl_review_notes"`
}

// CreateCommunityUpdateInput is the request body for
// POST /attestation/:id/updates.
type CreateCommunityUpdateInput struct {
	StepIndex      *int32 `json:"step_index"`
	FeeEstimate    string `json:"fee_estimate"`
	TurnaroundDays *int32 `json:"turnaround_days"`
	Note           string `json:"note"`
}

// GeneratePathwayInput is the request body for the Content Agent draft
// placeholder endpoint (POST /admin/attestation/generate).
type GeneratePathwayInput struct {
	DocumentType       string `json:"document_type"`
	IssuingSpaceID     string `json:"issuing_space_id"`
	DestinationSpaceID string `json:"destination_space_id"`
}

// Service holds the business logic for the Attestation Guide.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates an attestation guide service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// Enabled reads the attestation_guide.enabled system_setting toggle
// (§97 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "attestation_guide.enabled", false)
}

// List returns a paginated, filtered list of published attestation
// pathways. Anonymous-accessible.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID, issuingSpaceID pgtype.UUID, documentType string, page, limit int32) ([]PathwayDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.ListPathways(ctx, ListPathwaysFilter{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		DocumentType:       documentType,
		IssuingSpaceID:     issuingSpaceID,
		Status:             PathwayStatusPublished,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("attestationguide: list: %w", err)
	}

	dtos := make([]PathwayDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, pathwayRowToDTO(r))
	}
	return dtos, total, nil
}

// GetByID returns a single published attestation pathway by id.
// Anonymous-accessible.
func (s *Service) GetByID(ctx context.Context, id, tenantID pgtype.UUID) (PathwayDTO, error) {
	row, err := s.store.GetPathwayByID(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PathwayDTO{}, pgx.ErrNoRows
		}
		return PathwayDTO{}, fmt.Errorf("attestationguide: get by id: %w", err)
	}
	return pathwayRowToDTO(row), nil
}

// Create creates a new attestation pathway. Admin-only
// (attestation_guide.manage permission checked at route level).
func (s *Service) Create(ctx context.Context, tenantID pgtype.UUID, input CreatePathwayInput) (PathwayDTO, error) {
	if input.DocumentType == "" {
		return PathwayDTO{}, fmt.Errorf("document_type is required")
	}
	if input.IssuingSpaceID == "" {
		return PathwayDTO{}, fmt.Errorf("issuing_space_id is required")
	}
	if input.DestinationSpaceID == "" {
		return PathwayDTO{}, fmt.Errorf("destination_space_id is required")
	}

	var issuingSpaceID pgtype.UUID
	if err := issuingSpaceID.Scan(input.IssuingSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid issuing_space_id: %w", err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return PathwayDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	steps := []byte(input.PathwaySteps)
	if len(steps) == 0 {
		steps = []byte("[]")
	}

	p := Pathway{
		TenantID:           tenantID,
		DocumentType:       input.DocumentType,
		IssuingSpaceID:     issuingSpaceID,
		DestinationSpaceID: destSpaceID,
		ApostilleRequired:  input.ApostilleRequired,
		PathwaySteps:       steps,
		Status:             input.Status,
		IsActive:           true,
	}
	if input.IsActive != nil {
		p.IsActive = *input.IsActive
	}
	if input.AIModel != "" {
		p.AIModel = &input.AIModel
	}
	if input.AIPromptHash != "" {
		p.AIPromptHash = &input.AIPromptHash
	}
	if input.HITLReviewNotes != "" {
		p.HITLReviewNotes = &input.HITLReviewNotes
	}
	if input.CommunityLastVerified != "" {
		if err := p.CommunityLastVerified.Scan(input.CommunityLastVerified); err != nil {
			return PathwayDTO{}, fmt.Errorf("invalid community_last_verified: %w", err)
		}
	}

	created, err := s.store.CreatePathway(ctx, p)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: create: %w", err)
	}

	row, err := s.store.GetPathwayByID(ctx, created.ID, tenantID)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: create: reload: %w", err)
	}
	return pathwayRowToDTO(row), nil
}

// Approve performs the HITL approval on an AI-drafted pathway. Sets
// hitl_reviewed_by, hitl_reviewed_at = now(), status = 'published'.
// Admin-only (attestation_guide.manage permission).
func (s *Service) Approve(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, notes string) (PathwayDTO, error) {
	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}
	if err := s.store.ApprovePathway(ctx, id, tenantID, reviewerID, notesPtr); err != nil {
		if err == pgx.ErrNoRows {
			return PathwayDTO{}, pgx.ErrNoRows
		}
		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: %w", err)
	}

	row, err := s.store.GetPathwayByID(ctx, id, tenantID)
	if err != nil {
		return PathwayDTO{}, fmt.Errorf("attestationguide: approve: reload: %w", err)
	}
	return pathwayRowToDTO(row), nil
}

// SubmitCommunityUpdate creates a pending community update for a
// pathway. Auth required (attestation_guide.update permission).
func (s *Service) SubmitCommunityUpdate(ctx context.Context, pathwayID, tenantID, userID pgtype.UUID, input CreateCommunityUpdateInput) (CommunityUpdateDTO, error) {
	// Verify the pathway exists and belongs to the tenant.
	if _, err := s.store.GetPathwayByID(ctx, pathwayID, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return CommunityUpdateDTO{}, pgx.ErrNoRows
		}
		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: submit update: lookup pathway: %w", err)
	}

	u := CommunityUpdate{
		PathwayID:      pathwayID,
		UserID:         userID,
		StepIndex:      input.StepIndex,
		TurnaroundDays: input.TurnaroundDays,
		Status:         UpdateStatusPending,
	}
	if input.FeeEstimate != "" {
		u.FeeEstimate = &input.FeeEstimate
	}
	if input.Note != "" {
		u.Note = &input.Note
	}

	created, err := s.store.CreateCommunityUpdate(ctx, u)
	if err != nil {
		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: submit update: %w", err)
	}
	return communityUpdateToDTO(created), nil
}

// ReviewCommunityUpdate approves or rejects a pending community update.
// Admin-only (attestation_guide.manage permission).
func (s *Service) ReviewCommunityUpdate(ctx context.Context, updateID, reviewerID pgtype.UUID, approve bool) (CommunityUpdateDTO, error) {
	status := UpdateStatusApproved
	if !approve {
		status = UpdateStatusRejected
	}
	if err := s.store.ReviewCommunityUpdate(ctx, updateID, reviewerID, status); err != nil {
		if err == pgx.ErrNoRows {
			return CommunityUpdateDTO{}, pgx.ErrNoRows
		}
		return CommunityUpdateDTO{}, fmt.Errorf("attestationguide: review update: %w", err)
	}
	dto := CommunityUpdateDTO{Status: status}
	if updateID.Valid {
		dto.ID = updateID.String()
	}
	return dto, nil
}

// AdminList returns all attestation pathways (any status) for the
// moderation queue. Admin-only (attestation_guide.manage permission).
func (s *Service) AdminList(ctx context.Context, tenantID, destSpaceID pgtype.UUID, status, documentType string, isActive *bool, page, limit int32) ([]PathwayDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{
		TenantID:           tenantID,
		Status:             status,
		DocumentType:       documentType,
		DestinationSpaceID: destSpaceID,
		IsActive:           isActive,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("attestationguide: admin list: %w", err)
	}

	dtos := make([]PathwayDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, pathwayRowToDTO(r))
	}
	return dtos, total, nil
}

// pathwayRowToDTO converts a PathwayRow (joined query result) to a
// PathwayDTO.
func pathwayRowToDTO(r PathwayRow) PathwayDTO {
	dto := PathwayDTO{
		ID:                r.ID.String(),
		DocumentType:      r.DocumentType,
		ApostilleRequired: r.ApostilleRequired,
		PathwaySteps:      json.RawMessage(r.PathwaySteps),
		Status:            r.Status,
		IsActive:          r.IsActive,
		AIGenerated:       r.AIGenerated,
		IsJapabotIndexed:  r.IsJapabotIndexed,
		CreatedAt:         formatTs(r.CreatedAt),
		UpdatedAt:         formatTs(r.UpdatedAt),
	}
	if r.IssuingSpaceID.Valid {
		dto.IssuingSpaceID = r.IssuingSpaceID.String()
	}
	if r.IssuingSpaceName != nil {
		dto.IssuingSpaceName = *r.IssuingSpaceName
	}
	if r.IssuingSpaceCode != nil {
		dto.IssuingSpaceCode = *r.IssuingSpaceCode
	}
	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}
	if r.DestSpaceName != nil {
		dto.DestinationSpaceName = *r.DestSpaceName
	}
	if r.DestSpaceCode != nil {
		dto.DestinationSpaceCode = *r.DestSpaceCode
	}
	if r.CommunityLastVerified.Valid {
		dto.CommunityLastVerified = r.CommunityLastVerified.Time.Format("2006-01-02")
	}
	if r.AIModel != nil {
		dto.AIModel = *r.AIModel
	}
	if r.AIPromptHash != nil {
		dto.AIPromptHash = *r.AIPromptHash
	}
	if r.HITLReviewedBy.Valid {
		dto.HITLReviewedBy = r.HITLReviewedBy.String()
	}
	dto.HITLReviewedAt = formatTs(r.HITLReviewedAt)
	if r.HITLReviewNotes != nil {
		dto.HITLReviewNotes = *r.HITLReviewNotes
	}
	return dto
}

// communityUpdateToDTO converts a CommunityUpdate row to a DTO.
func communityUpdateToDTO(u CommunityUpdate) CommunityUpdateDTO {
	dto := CommunityUpdateDTO{
		Status:    u.Status,
		CreatedAt: formatTs(u.CreatedAt),
	}
	if u.ID.Valid {
		dto.ID = u.ID.String()
	}
	if u.PathwayID.Valid {
		dto.PathwayID = u.PathwayID.String()
	}
	if u.UserID.Valid {
		dto.UserID = u.UserID.String()
	}
	dto.StepIndex = u.StepIndex
	if u.FeeEstimate != nil {
		dto.FeeEstimate = *u.FeeEstimate
	}
	dto.TurnaroundDays = u.TurnaroundDays
	if u.Note != nil {
		dto.Note = *u.Note
	}
	if u.ReviewedBy.Valid {
		dto.ReviewedBy = u.ReviewedBy.String()
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
