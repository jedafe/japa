// Package taxcalculator — service.go orchestrates public lookup, admin
// CRUD, the Content Agent draft placeholder, and Human-in-the-Loop
// approval for tax obligation guides and treaty pairs. Feature-flagged
// via tax_calculator.enabled system_setting (§100).
//
// Ref: japaconnect-engineering/sections/100-tax-calculator.md
//
//	japaconnect-engineering/adr/ADR-108-tax-calculator.md
package taxcalculator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// GuideDTO is the API response shape for a single tax obligation guide.
// ActionChecklist is exposed as json.RawMessage so the JSONB payload is
// passed through verbatim.
type GuideDTO struct {
	ID                   string          `json:"id"`
	OriginSpaceID        string          `json:"origin_space_id"`
	OriginSpaceName      string          `json:"origin_space_name,omitempty"`
	DestinationSpaceID   string          `json:"destination_space_id"`
	DestinationSpaceName string          `json:"destination_space_name,omitempty"`
	EmploymentStatus     string          `json:"employment_status"`
	ResidencyTrigger     string          `json:"residency_trigger,omitempty"`
	WorldwideIncomeTaxed *bool           `json:"worldwide_income_taxed,omitempty"`
	FirstYearObligations string          `json:"first_year_obligations,omitempty"`
	ActionChecklist      json.RawMessage `json:"action_checklist,omitempty"`
	Status               string          `json:"status"`
	IsActive             bool            `json:"is_active"`
	AIGenerated          bool            `json:"ai_generated"`
	AIModel              string          `json:"ai_model,omitempty"`
	AIPromptHash         string          `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy       string          `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedAt       string          `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes      string          `json:"hitl_review_notes,omitempty"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
}

// TreatyDTO is the API response shape for a single tax treaty pair.
type TreatyDTO struct {
	ID                   string `json:"id"`
	OriginSpaceID        string `json:"origin_space_id"`
	OriginSpaceName      string `json:"origin_space_name,omitempty"`
	DestinationSpaceID   string `json:"destination_space_id"`
	DestinationSpaceName string `json:"destination_space_name,omitempty"`
	TreatyExists         bool   `json:"treaty_exists"`
	TreatySummary        string `json:"treaty_summary,omitempty"`
	TreatySourceURL      string `json:"treaty_source_url,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

// CreateGuideRequest is the request body for POST /admin/tax-calculator/guides.
type CreateGuideRequest struct {
	OriginSpaceID        string          `json:"origin_space_id"`
	DestinationSpaceID   string          `json:"destination_space_id"`
	EmploymentStatus     string          `json:"employment_status"`
	ResidencyTrigger     string          `json:"residency_trigger"`
	WorldwideIncomeTaxed *bool           `json:"worldwide_income_taxed"`
	FirstYearObligations string          `json:"first_year_obligations"`
	ActionChecklist      json.RawMessage `json:"action_checklist"`
	Status               string          `json:"status"`
	IsActive             *bool           `json:"is_active"`
}

// UpdateGuideRequest is the request body for PATCH /admin/tax-calculator/guides/:id.
type UpdateGuideRequest struct {
	ResidencyTrigger     *string         `json:"residency_trigger"`
	WorldwideIncomeTaxed *bool           `json:"worldwide_income_taxed"`
	FirstYearObligations *string         `json:"first_year_obligations"`
	ActionChecklist      json.RawMessage `json:"action_checklist"`
	Status               *string         `json:"status"`
	IsActive             *bool           `json:"is_active"`
}

// ApproveGuideRequest is the request body for PATCH
// /admin/tax-calculator/guides/:id/approve.
type ApproveGuideRequest struct {
	Notes *string `json:"notes"`
}

// GenerateGuideRequest is the request body for POST
// /admin/tax-calculator/guides/generate (Content Agent draft).
type GenerateGuideRequest struct {
	OriginSpaceID      string `json:"origin_space_id"`
	DestinationSpaceID string `json:"destination_space_id"`
	EmploymentStatus   string `json:"employment_status"`
	Prompt             string `json:"prompt"`
}

// GenerateGuideResult is the response for the generate endpoint. The
// Content Agent draft is saved in 'ai_draft' status and must be
// approved via the HITL approve endpoint before publication.
type GenerateGuideResult struct {
	Status             string `json:"status"`
	Message            string `json:"message"`
	GuideID            string `json:"guide_id"`
	OriginSpaceID      string `json:"origin_space_id"`
	DestinationSpaceID string `json:"destination_space_id"`
	EmploymentStatus   string `json:"employment_status"`
}

// UpsertTreatyRequest is the request body for POST /admin/tax-calculator/treaties.
type UpsertTreatyRequest struct {
	OriginSpaceID      string `json:"origin_space_id"`
	DestinationSpaceID string `json:"destination_space_id"`
	TreatyExists       bool   `json:"treaty_exists"`
	TreatySummary      string `json:"treaty_summary"`
	TreatySourceURL    string `json:"treaty_source_url"`
}

// Service holds the business logic for the Tax Calculator.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a tax calculator service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// Enabled reads the tax_calculator.enabled system_setting toggle
// (§100 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, tenantID, "tax_calculator.enabled", false)
}

// List returns a paginated, filtered list of published, active tax
// obligation guides. Anonymous-accessible.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID, originSpaceID pgtype.UUID, employmentStatus string, page, limit int32) ([]GuideDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListGuides(ctx, ListGuidesFilter{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		OriginSpaceID:      originSpaceID,
		EmploymentStatus:   employmentStatus,
		OnlyActive:         true,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("taxcalculator: list: %w", err)
	}

	dtos := make([]GuideDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, guideRowToDTO(r))
	}

	return dtos, total, nil
}

// GetTreaty returns the treaty status for an origin/destination pair.
// Anonymous-accessible. Returns pgx.ErrNoRows when no treaty record exists.
func (s *Service) GetTreaty(ctx context.Context, tenantID, originSpaceID, destSpaceID pgtype.UUID) (TreatyDTO, error) {
	row, err := s.store.GetTreatyByPair(ctx, tenantID, originSpaceID, destSpaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreatyDTO{}, pgx.ErrNoRows
		}

		return TreatyDTO{}, fmt.Errorf("taxcalculator: get treaty: %w", err)
	}

	return treatyRowToDTO(row), nil
}

// CreateGuide creates a new tax obligation guide (admin). The caller must
// have the tax_calculator.manage permission (checked at route level).
func (s *Service) CreateGuide(ctx context.Context, tenantID pgtype.UUID, req CreateGuideRequest) (GuideDTO, error) {
	if req.EmploymentStatus == "" {
		return GuideDTO{}, errors.New("employment_status is required")
	}

	if !validEmploymentStatus(req.EmploymentStatus) {
		return GuideDTO{}, errors.New("invalid employment_status")
	}

	var originSpaceID pgtype.UUID
	if err := originSpaceID.Scan(req.OriginSpaceID); err != nil {
		return GuideDTO{}, fmt.Errorf("invalid origin_space_id: %w", err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(req.DestinationSpaceID); err != nil {
		return GuideDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	checklist, err := normalizeChecklist(req.ActionChecklist)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("invalid action_checklist: %w", err)
	}

	in := CreateGuideInput{
		TenantID:           tenantID,
		OriginSpaceID:      originSpaceID,
		DestinationSpaceID: destSpaceID,
		EmploymentStatus:   req.EmploymentStatus,
		ActionChecklist:    checklist,
		Status:             req.Status,
	}
	if req.ResidencyTrigger != "" {
		in.ResidencyTrigger = &req.ResidencyTrigger
	}

	if req.WorldwideIncomeTaxed != nil {
		in.WorldwideIncomeTaxed = req.WorldwideIncomeTaxed
	}

	if req.FirstYearObligations != "" {
		in.FirstYearObligations = &req.FirstYearObligations
	}

	if req.IsActive != nil {
		in.IsActive = *req.IsActive
	} else {
		in.IsActive = true
	}

	row, err := s.store.CreateGuide(ctx, in)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("taxcalculator: create guide: %w", err)
	}

	return guideRowToDTO(row), nil
}

// UpdateGuide updates the mutable fields on an existing guide (admin).
func (s *Service) UpdateGuide(ctx context.Context, id, tenantID pgtype.UUID, req UpdateGuideRequest) (GuideDTO, error) {
	var checklist []byte

	if req.ActionChecklist != nil {
		c, err := normalizeChecklist(req.ActionChecklist)
		if err != nil {
			return GuideDTO{}, fmt.Errorf("invalid action_checklist: %w", err)
		}

		checklist = c
	}

	if err := s.store.UpdateGuide(ctx, id, tenantID, UpdateGuideInput{
		ResidencyTrigger:     req.ResidencyTrigger,
		WorldwideIncomeTaxed: req.WorldwideIncomeTaxed,
		FirstYearObligations: req.FirstYearObligations,
		ActionChecklist:      checklist,
		Status:               req.Status,
		IsActive:             req.IsActive,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("taxcalculator: update guide: %w", err)
	}

	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("taxcalculator: update guide: reload: %w", err)
	}

	return guideRowToDTO(row), nil
}

// DeleteGuide deletes a tax obligation guide by id and tenant_id (admin).
func (s *Service) DeleteGuide(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.DeleteGuide(ctx, id, tenantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("taxcalculator: delete guide: %w", err)
	}

	return nil
}

// ApproveGuide performs the Human-in-the-Loop approval for a guide
// (admin). Sets hitl_reviewed_by, hitl_reviewed_at = now(), and
// status = 'published'.
func (s *Service) ApproveGuide(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, req ApproveGuideRequest) (GuideDTO, error) {
	var notes *string
	if req.Notes != nil && *req.Notes != "" {
		notes = req.Notes
	}

	if err := s.store.ApproveGuide(ctx, id, tenantID, reviewerID, notes); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideDTO{}, pgx.ErrNoRows
		}

		return GuideDTO{}, fmt.Errorf("taxcalculator: approve guide: %w", err)
	}

	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("taxcalculator: approve guide: reload: %w", err)
	}

	return guideRowToDTO(row), nil
}

// GenerateGuide creates a Content Agent draft guide in 'ai_draft' status
// (§100, ADR-088). The draft is marked ai_generated=true with
// ai_model='content_agent_v1' and ai_prompt_hash=SHA256(prompt). It
// requires HITL approval (ApproveGuide) before publication. Admin-only
// (tax_calculator.manage permission checked at route level).
func (s *Service) GenerateGuide(ctx context.Context, tenantID pgtype.UUID, req GenerateGuideRequest) (GenerateGuideResult, error) {
	if req.EmploymentStatus == "" {
		return GenerateGuideResult{}, errors.New("employment_status is required")
	}

	if !validEmploymentStatus(req.EmploymentStatus) {
		return GenerateGuideResult{}, errors.New("invalid employment_status")
	}

	if req.OriginSpaceID == "" {
		return GenerateGuideResult{}, errors.New("origin_space_id is required")
	}

	if req.DestinationSpaceID == "" {
		return GenerateGuideResult{}, errors.New("destination_space_id is required")
	}

	var originSpaceID pgtype.UUID
	if err := originSpaceID.Scan(req.OriginSpaceID); err != nil {
		return GenerateGuideResult{}, fmt.Errorf("invalid origin_space_id: %w", err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(req.DestinationSpaceID); err != nil {
		return GenerateGuideResult{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	// Compute ai_prompt_hash = SHA256(prompt) for audit reproducibility
	// (ADR-088).
	promptHash := computePromptHash(req.Prompt)
	aiModel := "content_agent_v1"

	row, err := s.store.CreateGuide(ctx, CreateGuideInput{
		TenantID:             tenantID,
		OriginSpaceID:        originSpaceID,
		DestinationSpaceID:   destSpaceID,
		EmploymentStatus:     req.EmploymentStatus,
		ResidencyTrigger:     strPtr("AI-generated — pending review"),
		FirstYearObligations: strPtr("AI-generated first-year obligations — pending HITL review."),
		ActionChecklist:      []byte("[]"),
		Status:               GuideStatusAIDraft,
		IsActive:             false,
		AIGenerated:          true,
		AIModel:              &aiModel,
		AIPromptHash:         &promptHash,
	})
	if err != nil {
		return GenerateGuideResult{}, fmt.Errorf("taxcalculator: generate guide: %w", err)
	}

	slog.Info(
		"taxcalculator: content agent draft created",
		"tenant_id", tenantID,
		"guide_id", row.ID.String(),
		"origin_space_id", req.OriginSpaceID,
		"destination_space_id", req.DestinationSpaceID,
		"employment_status", req.EmploymentStatus,
	)

	return GenerateGuideResult{
		Status:             GuideStatusAIDraft,
		Message:            "Content Agent draft created in 'ai_draft' status. Review and approve to publish.",
		GuideID:            row.ID.String(),
		OriginSpaceID:      req.OriginSpaceID,
		DestinationSpaceID: req.DestinationSpaceID,
		EmploymentStatus:   req.EmploymentStatus,
	}, nil
}

// UpsertTreaty creates or updates a tax treaty pair (admin). Uses
// ON CONFLICT DO UPDATE so repeated calls for the same pair update the
// mutable fields.
func (s *Service) UpsertTreaty(ctx context.Context, tenantID pgtype.UUID, req UpsertTreatyRequest) (TreatyDTO, error) {
	var originSpaceID pgtype.UUID
	if err := originSpaceID.Scan(req.OriginSpaceID); err != nil {
		return TreatyDTO{}, fmt.Errorf("invalid origin_space_id: %w", err)
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(req.DestinationSpaceID); err != nil {
		return TreatyDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	in := TreatyUpsertInput{
		TenantID:           tenantID,
		OriginSpaceID:      originSpaceID,
		DestinationSpaceID: destSpaceID,
		TreatyExists:       req.TreatyExists,
	}
	if req.TreatySummary != "" {
		in.TreatySummary = &req.TreatySummary
	}

	if req.TreatySourceURL != "" {
		in.TreatySourceURL = &req.TreatySourceURL
	}

	row, err := s.store.CreateOrUpdateTreaty(ctx, in)
	if err != nil {
		return TreatyDTO{}, fmt.Errorf("taxcalculator: upsert treaty: %w", err)
	}

	return treatyRowToDTO(row), nil
}

// AdminList returns all guides (any status) plus all treaty pairs for the
// admin management view.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status, employmentStatus, q string, destSpaceID, originSpaceID pgtype.UUID, page, limit int32) ([]GuideDTO, []TreatyDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	guides, treaties, total, err := s.store.AdminList(ctx, AdminListFilter{
		TenantID:           tenantID,
		Status:             status,
		EmploymentStatus:   employmentStatus,
		DestinationSpaceID: destSpaceID,
		OriginSpaceID:      originSpaceID,
		Q:                  q,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("taxcalculator: admin list: %w", err)
	}

	guideDTOs := make([]GuideDTO, 0, len(guides))
	for _, r := range guides {
		guideDTOs = append(guideDTOs, guideRowToDTO(r))
	}

	treatyDTOs := make([]TreatyDTO, 0, len(treaties))
	for _, r := range treaties {
		treatyDTOs = append(treatyDTOs, treatyRowToDTO(r))
	}

	return guideDTOs, treatyDTOs, total, nil
}

// validEmploymentStatus reports whether s is one of the allowed
// employment_status values (§100).
func validEmploymentStatus(s string) bool {
	switch s {
	case EmploymentEmployed, EmploymentSelfEmployed, EmploymentContractor, EmploymentUnemployed:
		return true
	default:
		return false
	}
}

// normalizeChecklist ensures the JSONB action_checklist is valid JSON and
// defaults to an empty array when nil/empty.
func normalizeChecklist(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return []byte("[]"), nil
	}

	if !json.Valid(raw) {
		return nil, errors.New("action_checklist is not valid JSON")
	}

	return raw, nil
}

// guideRowToDTO converts a GuideRow into a GuideDTO.
func guideRowToDTO(r GuideRow) GuideDTO {
	dto := GuideDTO{
		ID:                 r.ID.String(),
		OriginSpaceID:      r.OriginSpaceID.String(),
		DestinationSpaceID: r.DestinationSpaceID.String(),
		EmploymentStatus:   r.EmploymentStatus,
		Status:             r.Status,
		IsActive:           r.IsActive,
		AIGenerated:        r.AIGenerated,
		CreatedAt:          formatTs(r.CreatedAt),
		UpdatedAt:          formatTs(r.UpdatedAt),
	}
	if r.OriginSpaceName != nil {
		dto.OriginSpaceName = *r.OriginSpaceName
	}

	if r.DestinationSpaceName != nil {
		dto.DestinationSpaceName = *r.DestinationSpaceName
	}

	if r.ResidencyTrigger != nil {
		dto.ResidencyTrigger = *r.ResidencyTrigger
	}

	if r.WorldwideIncomeTaxed != nil {
		dto.WorldwideIncomeTaxed = r.WorldwideIncomeTaxed
	}

	if r.FirstYearObligations != nil {
		dto.FirstYearObligations = *r.FirstYearObligations
	}

	if len(r.ActionChecklist) > 0 && string(r.ActionChecklist) != "null" {
		dto.ActionChecklist = json.RawMessage(r.ActionChecklist)
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

// treatyRowToDTO converts a TreatyRow into a TreatyDTO.
func treatyRowToDTO(r TreatyRow) TreatyDTO {
	dto := TreatyDTO{
		ID:                 r.ID.String(),
		OriginSpaceID:      r.OriginSpaceID.String(),
		DestinationSpaceID: r.DestinationSpaceID.String(),
		TreatyExists:       r.TreatyExists,
		CreatedAt:          formatTs(r.CreatedAt),
		UpdatedAt:          formatTs(r.UpdatedAt),
	}
	if r.OriginSpaceName != nil {
		dto.OriginSpaceName = *r.OriginSpaceName
	}

	if r.DestinationSpaceName != nil {
		dto.DestinationSpaceName = *r.DestinationSpaceName
	}

	if r.TreatySummary != nil {
		dto.TreatySummary = *r.TreatySummary
	}

	if r.TreatySourceURL != nil {
		dto.TreatySourceURL = *r.TreatySourceURL
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

// computePromptHash returns the hex-encoded SHA-256 of the prompt string
// for audit reproducibility (ADR-088).
func computePromptHash(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

// strPtr returns a pointer to s. Convenience for optional string fields.
func strPtr(s string) *string {
	return &s
}
