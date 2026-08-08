// Package housingguide — service.go orchestrates housing guide listing,
// retrieval, admin CRUD, Content Agent draft generation, and HITL
// approval. Feature-flagged via housing_guide.enabled system_setting
// (§96, ADR-104).
//
// Ref: japaconnect-engineering/sections/96-housing-guide.md
//      japaconnect-engineering/adr/ADR-104-housing-guide.md
package housingguide

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// GuideDTO is the API response shape for a single housing guide.
type GuideDTO struct {
	ID                   string          `json:"id"`
	DestinationSpaceID   string          `json:"destination_space_id"`
	CitySpaceID          string          `json:"city_space_id,omitempty"`
	AccommodationType    string          `json:"accommodation_type,omitempty"`
	SearchPlatforms      json.RawMessage `json:"search_platforms,omitempty"`
	LandlordRequirements string          `json:"landlord_requirements,omitempty"`
	RentRanges           json.RawMessage `json:"rent_ranges,omitempty"`
	ScamCategories       []string        `json:"scam_categories,omitempty"`
	Tips                 string          `json:"tips,omitempty"`
	Status               string          `json:"status"`
	IsActive             bool            `json:"is_active"`
	AIGenerated          bool            `json:"ai_generated"`
	AIModel              string          `json:"ai_model,omitempty"`
	AIPromptHash         string          `json:"ai_prompt_hash,omitempty"`
	HITLReviewedBy       string          `json:"hitl_reviewed_by,omitempty"`
	HITLReviewedAt       string          `json:"hitl_reviewed_at,omitempty"`
	HITLReviewNotes      string          `json:"hitl_review_notes,omitempty"`
	LastVerifiedAt       string          `json:"last_verified_at,omitempty"`
	IsJapabotIndexed     bool            `json:"is_japabot_indexed"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	DestSpaceName        string          `json:"destination_space_name,omitempty"`
	DestSpaceCode        string          `json:"destination_space_code,omitempty"`
	CitySpaceName        string          `json:"city_space_name,omitempty"`
	CitySpaceCode        string          `json:"city_space_code,omitempty"`
	ReviewerName         string          `json:"reviewer_name,omitempty"`
}

// CreateGuideRequest is the request body for POST /admin/housing-guides.
type CreateGuideRequest struct {
	DestinationSpaceID   string          `json:"destination_space_id"`
	CitySpaceID          string          `json:"city_space_id"`
	AccommodationType    string          `json:"accommodation_type"`
	SearchPlatforms      json.RawMessage `json:"search_platforms"`
	LandlordRequirements string          `json:"landlord_requirements"`
	RentRanges           json.RawMessage `json:"rent_ranges"`
	ScamCategories       []string        `json:"scam_categories"`
	Tips                 string          `json:"tips"`
	Status               string          `json:"status"`
	IsActive             *bool           `json:"is_active"`
	AIModel              string          `json:"ai_model"`
	AIPromptHash         string          `json:"ai_prompt_hash"`
}

// UpdateGuideRequest is the request body for PATCH /admin/housing-guides/:id.
// All fields are optional; only provided fields are applied.
type UpdateGuideRequest struct {
	AccommodationType    *string         `json:"accommodation_type"`
	SearchPlatforms      json.RawMessage `json:"search_platforms"`
	LandlordRequirements *string         `json:"landlord_requirements"`
	RentRanges           json.RawMessage `json:"rent_ranges"`
	ScamCategories       []string        `json:"scam_categories"`
	Tips                 *string         `json:"tips"`
	Status               *string         `json:"status"`
	IsActive             *bool           `json:"is_active"`
	AIModel              *string         `json:"ai_model"`
	AIPromptHash         *string         `json:"ai_prompt_hash"`
	IsJapabotIndexed     *bool           `json:"is_japabot_indexed"`
}

// ApproveGuideRequest is the request body for PATCH
// /admin/housing-guides/:id/approve.
type ApproveGuideRequest struct {
	Notes *string `json:"notes"`
}

// GenerateGuideRequest is the request body for POST
// /admin/housing-guides/generate (Content Agent draft placeholder).
type GenerateGuideRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	CitySpaceID        string `json:"city_space_id"`
	AccommodationType  string `json:"accommodation_type"`
}

// Service holds the business logic for the Housing & Accommodation Finder
// Guide.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a housing guide service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// Enabled reads the housing_guide.enabled system_setting toggle
// (§96 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "housing_guide.enabled", false)
}

// List returns a paginated, filtered list of published housing guides.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID, citySpaceID pgtype.UUID, accommodationType string, page, limit int32) ([]GuideDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	active := true
	rows, total, err := s.store.ListGuides(ctx, ListGuidesFilter{
		TenantID:          tenantID,
		DestSpaceID:       destSpaceID,
		CitySpaceID:       citySpaceID,
		AccommodationType: accommodationType,
		Status:            StatusPublished,
		IsActive:          &active,
		Limit:             limit,
		Offset:            offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("housingguide: list: %w", err)
	}

	dtos := make([]GuideDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, total, nil
}

// GetByID returns a single published housing guide by id.
func (s *Service) GetByID(ctx context.Context, id, tenantID pgtype.UUID) (GuideDTO, error) {
	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return GuideDTO{}, pgx.ErrNoRows
		}
		return GuideDTO{}, fmt.Errorf("housingguide: get by id: %w", err)
	}
	return rowToDTO(row), nil
}

// Create creates a new housing guide (admin). Returns the created guide.
func (s *Service) Create(ctx context.Context, tenantID pgtype.UUID, input CreateGuideRequest) (GuideDTO, error) {
	if input.DestinationSpaceID == "" {
		return GuideDTO{}, fmt.Errorf("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return GuideDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var citySpaceID pgtype.UUID
	if input.CitySpaceID != "" {
		if err := citySpaceID.Scan(input.CitySpaceID); err != nil {
			return GuideDTO{}, fmt.Errorf("invalid city_space_id: %w", err)
		}
	}

	searchPlatforms, err := normalizeJSONB(input.SearchPlatforms, []byte("[]"))
	if err != nil {
		return GuideDTO{}, fmt.Errorf("invalid search_platforms: %w", err)
	}
	rentRanges, err := normalizeJSONB(input.RentRanges, []byte("{}"))
	if err != nil {
		return GuideDTO{}, fmt.Errorf("invalid rent_ranges: %w", err)
	}

	storeInput := CreateGuideInput{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		CitySpaceID:        citySpaceID,
		SearchPlatforms:    searchPlatforms,
		RentRanges:         rentRanges,
		ScamCategories:     input.ScamCategories,
		IsActive:           true,
	}
	if input.AccommodationType != "" {
		storeInput.AccommodationType = &input.AccommodationType
	}
	if input.LandlordRequirements != "" {
		storeInput.LandlordRequirements = &input.LandlordRequirements
	}
	if input.Tips != "" {
		storeInput.Tips = &input.Tips
	}
	if input.Status != "" {
		storeInput.Status = input.Status
	}
	if input.IsActive != nil {
		storeInput.IsActive = *input.IsActive
	}
	if input.AIModel != "" {
		storeInput.AIModel = &input.AIModel
	}
	if input.AIPromptHash != "" {
		storeInput.AIPromptHash = &input.AIPromptHash
	}

	id, err := s.store.CreateGuide(ctx, storeInput)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("housingguide: create: %w", err)
	}

	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("housingguide: create: fetch created: %w", err)
	}
	return rowToDTO(row), nil
}

// Update updates an existing housing guide (admin). Only provided fields
// are applied.
func (s *Service) Update(ctx context.Context, id, tenantID pgtype.UUID, input UpdateGuideRequest) (GuideDTO, error) {
	storeInput := UpdateGuideInput{
		AccommodationType:    input.AccommodationType,
		LandlordRequirements: input.LandlordRequirements,
		Tips:                 input.Tips,
		Status:               input.Status,
		IsActive:             input.IsActive,
		AIModel:              input.AIModel,
		AIPromptHash:         input.AIPromptHash,
		IsJapabotIndexed:     input.IsJapabotIndexed,
		ScamCategories:       input.ScamCategories,
	}

	if input.SearchPlatforms != nil {
		sp, err := normalizeJSONB(input.SearchPlatforms, []byte("[]"))
		if err != nil {
			return GuideDTO{}, fmt.Errorf("invalid search_platforms: %w", err)
		}
		storeInput.SearchPlatforms = sp
	}
	if input.RentRanges != nil {
		rr, err := normalizeJSONB(input.RentRanges, []byte("{}"))
		if err != nil {
			return GuideDTO{}, fmt.Errorf("invalid rent_ranges: %w", err)
		}
		storeInput.RentRanges = rr
	}

	if err := s.store.UpdateGuide(ctx, id, tenantID, storeInput); err != nil {
		if err == pgx.ErrNoRows {
			return GuideDTO{}, pgx.ErrNoRows
		}
		return GuideDTO{}, fmt.Errorf("housingguide: update: %w", err)
	}

	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("housingguide: update: fetch updated: %w", err)
	}
	return rowToDTO(row), nil
}

// Approve performs HITL approval on a housing guide (admin). Sets
// hitl_reviewed_by, hitl_reviewed_at=now(), status='published', and
// optional review notes.
func (s *Service) Approve(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, input ApproveGuideRequest) (GuideDTO, error) {
	if err := s.store.ApproveGuide(ctx, id, tenantID, reviewerID, input.Notes); err != nil {
		if err == pgx.ErrNoRows {
			return GuideDTO{}, pgx.ErrNoRows
		}
		return GuideDTO{}, fmt.Errorf("housingguide: approve: %w", err)
	}

	row, err := s.store.GetGuideByID(ctx, id, tenantID)
	if err != nil {
		return GuideDTO{}, fmt.Errorf("housingguide: approve: fetch approved: %w", err)
	}
	return rowToDTO(row), nil
}

// AdminList returns all housing guides (any status) for the admin
// management queue.
func (s *Service) AdminList(ctx context.Context, tenantID pgtype.UUID, status, accommodationType, q string, destSpaceID, citySpaceID pgtype.UUID, isActive *bool, page, limit int32) ([]GuideDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.AdminList(ctx, AdminListFilter{
		TenantID:          tenantID,
		Status:            status,
		AccommodationType: accommodationType,
		DestSpaceID:       destSpaceID,
		CitySpaceID:       citySpaceID,
		Q:                 q,
		IsActive:          isActive,
		Limit:             limit,
		Offset:            offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("housingguide: admin list: %w", err)
	}

	dtos := make([]GuideDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, rowToDTO(r))
	}
	return dtos, total, nil
}

// GenerateDraft is the Content Agent draft placeholder. It does not yet
// invoke the Content Agent; instead it returns a placeholder response
// indicating the generation has been queued. The actual AI generation
// will be wired in a follow-up (§96 Content Agent integration).
func (s *Service) GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input GenerateGuideRequest) (map[string]any, error) {
	if input.DestinationSpaceID == "" {
		return nil, fmt.Errorf("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return nil, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var citySpaceID pgtype.UUID
	if input.CitySpaceID != "" {
		if err := citySpaceID.Scan(input.CitySpaceID); err != nil {
			return nil, fmt.Errorf("invalid city_space_id: %w", err)
		}
	}

	slog.Info("housingguide: generate draft placeholder", "tenant_id", tenantID, "dest_space_id", destSpaceID, "city_space_id", citySpaceID, "accommodation_type", input.AccommodationType)

	return map[string]any{
		"status":               "queued",
		"destination_space_id": input.DestinationSpaceID,
		"city_space_id":        input.CitySpaceID,
		"accommodation_type":   input.AccommodationType,
		"message":              "Content Agent draft generation queued; not yet implemented.",
	}, nil
}

// rowToDTO converts a GuideRow (joined query result) to a GuideDTO.
func rowToDTO(r GuideRow) GuideDTO {
	dto := GuideDTO{
		ID:                 r.ID.String(),
		DestinationSpaceID: r.DestinationSpaceID.String(),
		SearchPlatforms:    normalizeRawMessage(r.SearchPlatforms),
		RentRanges:         normalizeRawMessage(r.RentRanges),
		ScamCategories:     r.ScamCategories,
		Status:             r.Status,
		IsActive:           r.IsActive,
		AIGenerated:        r.AIGenerated,
		IsJapabotIndexed:   r.IsJapabotIndexed,
		CreatedAt:          formatTs(r.CreatedAt),
		UpdatedAt:          formatTs(r.UpdatedAt),
		HITLReviewedAt:     formatTs(r.HITLReviewedAt),
		LastVerifiedAt:     formatDate(r.LastVerifiedAt),
	}
	if r.CitySpaceID.Valid {
		dto.CitySpaceID = r.CitySpaceID.String()
	}
	if r.AccommodationType != nil {
		dto.AccommodationType = *r.AccommodationType
	}
	if r.LandlordRequirements != nil {
		dto.LandlordRequirements = *r.LandlordRequirements
	}
	if r.Tips != nil {
		dto.Tips = *r.Tips
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
	if r.HITLReviewNotes != nil {
		dto.HITLReviewNotes = *r.HITLReviewNotes
	}
	if r.DestSpaceName != nil {
		dto.DestSpaceName = *r.DestSpaceName
	}
	if r.DestSpaceCode != nil {
		dto.DestSpaceCode = *r.DestSpaceCode
	}
	if r.CitySpaceName != nil {
		dto.CitySpaceName = *r.CitySpaceName
	}
	if r.CitySpaceCode != nil {
		dto.CitySpaceCode = *r.CitySpaceCode
	}
	if r.ReviewerName != nil {
		dto.ReviewerName = *r.ReviewerName
	}
	return dto
}

// normalizeJSONB validates/normalizes a json.RawMessage input. If the
// input is empty, the fallback is returned. Invalid JSON returns an
// error.
func normalizeJSONB(in json.RawMessage, fallback []byte) ([]byte, error) {
	if len(in) == 0 {
		return fallback, nil
	}
	var v any
	if err := json.Unmarshal(in, &v); err != nil {
		return nil, err
	}
	return in, nil
}

// normalizeRawMessage returns the raw JSON bytes for the DTO, or nil if
// empty.
func normalizeRawMessage(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	return b
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// formatDate renders a pgtype.Date as YYYY-MM-DD, or "" if NULL.
func formatDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}
