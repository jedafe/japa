// Package cvformatter — service.go orchestrates CV standard management,
// session creation, and quota enforcement for the International CV /
// Resume Formatter (§95, ADR-103). Feature-flagged via the
// cv_formatter.enabled system_setting. The actual AI formatting is
// dispatched as an Asynq task by the caller; this service creates the
// session row and returns it for polling.
//
// Ref: japaconnect-engineering/sections/95-cv-formatter.md
//      japaconnect-engineering/adr/ADR-103-cv-formatter.md
package cvformatter

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// permissionChecker is the subset of rbac.RBAC the cvformatter service
// needs to check the cv_formatter.unlimited permission (premium bypass).
// Local interface so cvformatter does not import rbac directly.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID, tenantID pgtype.UUID, permKey string) bool
}

// StandardDTO is the API response shape for a single CV standard.
type StandardDTO struct {
	ID                    string   `json:"id"`
	DestinationSpaceID    string   `json:"destination_space_id"`
	DestinationSpaceName  string   `json:"destination_space_name,omitempty"`
	DestinationSpaceCode  string   `json:"destination_space_code,omitempty"`
	RecommendedLengthPage string   `json:"recommended_length_pages,omitempty"`
	PhotoPolicy           string   `json:"photo_policy,omitempty"`
	DateFormat            string   `json:"date_format,omitempty"`
	RecommendedSections   []string `json:"recommended_sections,omitempty"`
	ToneGuidance          string   `json:"tone_guidance,omitempty"`
	CommonPitfalls        string   `json:"common_pitfalls,omitempty"`
	CreatedAt             string   `json:"created_at"`
	UpdatedAt             string   `json:"updated_at"`
}

// SessionDTO is the API response shape for a single CV format session.
type SessionDTO struct {
	ID                 string `json:"id"`
	TenantID           string `json:"tenant_id"`
	UserID             string `json:"user_id"`
	OriginSpaceID      string `json:"origin_space_id,omitempty"`
	DestinationSpaceID string `json:"destination_space_id"`
	InputText          string `json:"input_text"`
	OutputText         string `json:"output_text,omitempty"`
	OutputFormat       string `json:"output_format"`
	Status             string `json:"status"`
	AIModel            string `json:"ai_model,omitempty"`
	TokensUsed         int32  `json:"tokens_used,omitempty"`
	CreatedAt          string `json:"created_at"`
	ExpiresAt          string `json:"expires_at"`
}

// FormatInput is the request body for POST /cv-formatter/format.
type FormatInput struct {
	DestinationSpaceID string `json:"destination_space_id"`
	OriginSpaceID      string `json:"origin_space_id"`
	InputText          string `json:"input_text"`
	OutputFormat       string `json:"output_format"`
	AIModel            string `json:"ai_model"`
}

// StandardInput is the request body for POST /admin/cv-formatter/standards.
type StandardInput struct {
	DestinationSpaceID    string   `json:"destination_space_id"`
	RecommendedLengthPage string   `json:"recommended_length_pages"`
	PhotoPolicy           string   `json:"photo_policy"`
	DateFormat            string   `json:"date_format"`
	RecommendedSections   []string `json:"recommended_sections"`
	ToneGuidance          string   `json:"tone_guidance"`
	CommonPitfalls        string   `json:"common_pitfalls"`
}

// ErrQuotaExceeded is returned when a standard member has reached the
// monthly format-request quota.
var ErrQuotaExceeded = fmt.Errorf("cv_formatter quota exceeded for this month")

// Service holds the business logic for the CV Formatter.
type Service struct {
	store    *Store
	settings *config.Settings
	rbac     permissionChecker
}

// NewService creates a CV formatter service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// SetRBAC injects the RBAC resolver used for the cv_formatter.unlimited
// permission check (premium quota bypass). Nil-safe.
func (s *Service) SetRBAC(r permissionChecker) { s.rbac = r }

// Enabled reads the cv_formatter.enabled system_setting toggle
// (§95 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "cv_formatter.enabled", false)
}

// CheckQuota returns nil if the user may submit a new format request,
// or ErrQuotaExceeded if the standard-member monthly quota has been
// reached. Premium users (cv_formatter.unlimited permission) bypass the
// quota. The quota limit is read from the cv_formatter.quota_standard
// system_setting (default 3).
func (s *Service) CheckQuota(ctx context.Context, userID, tenantID pgtype.UUID) error {
	if s.rbac != nil && s.rbac.HasPermission(ctx, userID, tenantID, "cv_formatter.unlimited") {
		return nil
	}
	limit := 3
	if s.settings != nil {
		limit = s.settings.GetIntOr(ctx, tenantID, "cv_formatter.quota_standard", 3)
	}
	if limit <= 0 {
		return nil
	}
	count, err := s.store.CountUserSessionsThisMonth(ctx, userID)
	if err != nil {
		return fmt.Errorf("cvformatter: check quota: %w", err)
	}
	if count >= int64(limit) {
		return ErrQuotaExceeded
	}
	return nil
}

// ListStandards returns all CV standards as DTOs.
func (s *Service) ListStandards(ctx context.Context) ([]StandardDTO, error) {
	rows, err := s.store.ListStandards(ctx)
	if err != nil {
		return nil, fmt.Errorf("cvformatter: list standards: %w", err)
	}
	dtos := make([]StandardDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, standardRowToDTO(r))
	}
	return dtos, nil
}

// GetStandardByDest returns the CV standard for the given destination
// space ID.
func (s *Service) GetStandardByDest(ctx context.Context, destSpaceID pgtype.UUID) (StandardDTO, error) {
	row, err := s.store.GetStandardByDest(ctx, destSpaceID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return StandardDTO{}, pgx.ErrNoRows
		}
		return StandardDTO{}, fmt.Errorf("cvformatter: get standard by dest: %w", err)
	}
	return standardRowToDTO(row), nil
}

// UpsertStandard creates or updates a CV standard for the given
// destination space. If a standard already exists for the destination,
// it is updated; otherwise a new row is inserted.
func (s *Service) UpsertStandard(ctx context.Context, input StandardInput) (StandardDTO, error) {
	if input.DestinationSpaceID == "" {
		return StandardDTO{}, fmt.Errorf("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return StandardDTO{}, fmt.Errorf("cvformatter: invalid destination_space_id: %w", err)
	}

	storeInput := StandardUpsert{
		DestinationSpaceID: destSpaceID,
	}
	if input.RecommendedLengthPage != "" {
		storeInput.RecommendedLengthPages = &input.RecommendedLengthPage
	}
	if input.PhotoPolicy != "" {
		storeInput.PhotoPolicy = &input.PhotoPolicy
	}
	if input.DateFormat != "" {
		storeInput.DateFormat = &input.DateFormat
	}
	if len(input.RecommendedSections) > 0 {
		storeInput.RecommendedSections = input.RecommendedSections
	}
	if input.ToneGuidance != "" {
		storeInput.ToneGuidance = &input.ToneGuidance
	}
	if input.CommonPitfalls != "" {
		storeInput.CommonPitfalls = &input.CommonPitfalls
	}

	// Try create first; fall back to update on conflict.
	row, err := s.store.CreateStandard(ctx, storeInput)
	if err != nil {
		// If the standard already exists for this destination, update it.
		if updateErr := s.store.UpdateStandard(ctx, storeInput); updateErr != nil {
			if updateErr == pgx.ErrNoRows {
				return StandardDTO{}, fmt.Errorf("cvformatter: upsert standard: create failed (%w) and update found no row", err)
			}
			return StandardDTO{}, fmt.Errorf("cvformatter: upsert standard: update: %w", updateErr)
		}
		row, err = s.store.GetStandardByDest(ctx, destSpaceID)
		if err != nil {
			return StandardDTO{}, fmt.Errorf("cvformatter: upsert standard: refetch: %w", err)
		}
	}
	return standardRowToDTO(row), nil
}

// CreateSession creates a new CV format session with status='pending'.
// The caller must have already verified the feature flag and quota. The
// actual AI processing is dispatched as an Asynq task by the caller.
func (s *Service) CreateSession(ctx context.Context, tenantID, userID pgtype.UUID, input FormatInput) (SessionDTO, error) {
	if input.DestinationSpaceID == "" {
		return SessionDTO{}, fmt.Errorf("destination_space_id is required")
	}
	if input.InputText == "" {
		return SessionDTO{}, fmt.Errorf("input_text is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return SessionDTO{}, fmt.Errorf("cvformatter: invalid destination_space_id: %w", err)
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return SessionDTO{}, fmt.Errorf("cvformatter: invalid origin_space_id: %w", err)
		}
	}

	var aiModel *string
	if input.AIModel != "" {
		aiModel = &input.AIModel
	}

	row, err := s.store.CreateSession(ctx, SessionInput{
		TenantID:           tenantID,
		UserID:             userID,
		OriginSpaceID:      originSpaceID,
		DestinationSpaceID: destSpaceID,
		InputText:          input.InputText,
		OutputFormat:       input.OutputFormat,
		AIModel:            aiModel,
	})
	if err != nil {
		return SessionDTO{}, fmt.Errorf("cvformatter: create session: %w", err)
	}
	return sessionRowToDTO(row), nil
}

// GetSession returns a single CV format session by ID. The caller must
// verify ownership (user_id match) before returning the DTO to the
// client.
func (s *Service) GetSession(ctx context.Context, id pgtype.UUID) (SessionRow, error) {
	row, err := s.store.GetSessionByID(ctx, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return SessionRow{}, pgx.ErrNoRows
		}
		return SessionRow{}, fmt.Errorf("cvformatter: get session: %w", err)
	}
	return row, nil
}

// standardRowToDTO converts a StandardRow to a StandardDTO.
func standardRowToDTO(r StandardRow) StandardDTO {
	dto := StandardDTO{
		ID:                  r.ID.String(),
		DestinationSpaceID:  r.DestinationSpaceID.String(),
		RecommendedSections: r.RecommendedSections,
		CreatedAt:           formatTs(r.CreatedAt),
		UpdatedAt:           formatTs(r.UpdatedAt),
	}
	if r.DestinationSpaceName != nil {
		dto.DestinationSpaceName = *r.DestinationSpaceName
	}
	if r.DestinationSpaceCode != nil {
		dto.DestinationSpaceCode = *r.DestinationSpaceCode
	}
	if r.RecommendedLengthPages != nil {
		dto.RecommendedLengthPage = *r.RecommendedLengthPages
	}
	if r.PhotoPolicy != nil {
		dto.PhotoPolicy = *r.PhotoPolicy
	}
	if r.DateFormat != nil {
		dto.DateFormat = *r.DateFormat
	}
	if r.ToneGuidance != nil {
		dto.ToneGuidance = *r.ToneGuidance
	}
	if r.CommonPitfalls != nil {
		dto.CommonPitfalls = *r.CommonPitfalls
	}
	return dto
}

// sessionRowToDTO converts a SessionRow to a SessionDTO.
func sessionRowToDTO(r SessionRow) SessionDTO {
	dto := SessionDTO{
		ID:                 r.ID.String(),
		TenantID:           r.TenantID.String(),
		UserID:             r.UserID.String(),
		DestinationSpaceID: r.DestinationSpaceID.String(),
		InputText:          r.InputText,
		OutputFormat:       r.OutputFormat,
		Status:             r.Status,
		CreatedAt:          formatTs(r.CreatedAt),
		ExpiresAt:          formatTs(r.ExpiresAt),
	}
	if r.OriginSpaceID.Valid {
		dto.OriginSpaceID = r.OriginSpaceID.String()
	}
	if r.OutputText != nil {
		dto.OutputText = *r.OutputText
	}
	if r.AIModel != nil {
		dto.AIModel = *r.AIModel
	}
	if r.TokensUsed != nil {
		dto.TokensUsed = *r.TokensUsed
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
