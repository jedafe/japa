// Package japastage — service.go orchestrates japa stage profile reads,
// stage updates, visibility toggles, and the public stage directory. It is
// feature-flagged via the japa_stage.enabled system_setting (§87).
//
// Ref: japaconnect-engineering/sections/87-japa-stage-progress-profiles.md
//      japaconnect-engineering/adr/ADR-095-japa-stage-progress-profiles.md
package japastage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// ProfileDTO is the API response shape for a single japa stage profile
// (own view — includes history and visibility flag).
type ProfileDTO struct {
	UserID         string          `json:"user_id"`
	Username       string          `json:"username,omitempty"`
	DisplayName    string          `json:"display_name,omitempty"`
	AvatarURL      string          `json:"avatar_url,omitempty"`
	Stage          string          `json:"stage,omitempty"`
	DestSpaceID    string          `json:"destination_space_id,omitempty"`
	DestSpaceName  string          `json:"destination_space_name,omitempty"`
	DestSpaceCode  string          `json:"destination_space_code,omitempty"`
	IsPublic       bool            `json:"is_public"`
	History        json.RawMessage `json:"history,omitempty"`
	UpdatedAt      string          `json:"updated_at,omitempty"`
}

// PublicProfileDTO is the trimmed shape returned for the public view and
// the stage directory. History and the visibility flag are omitted.
type PublicProfileDTO struct {
	UserID        string `json:"user_id"`
	Username      string `json:"username,omitempty"`
	DisplayName   string `json:"display_name,omitempty"`
	AvatarURL     string `json:"avatar_url,omitempty"`
	Stage         string `json:"stage"`
	DestSpaceID   string `json:"destination_space_id,omitempty"`
	DestSpaceName string `json:"destination_space_name,omitempty"`
	DestSpaceCode string `json:"destination_space_code,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

// UpdateStageInput is the request body for PATCH /me/japa-stage.
type UpdateStageInput struct {
	Stage       string `json:"stage"`
	DestSpaceID string `json:"destination_space_id"`
}

// SetPublicInput is the request body for toggling visibility.
type SetPublicInput struct {
	IsPublic bool `json:"is_public"`
}

// Service holds the business logic for Japa Stage Progress Profiles.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a japa stage service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{
		store:    store,
		settings: settings,
	}
}

// Enabled reads the japa_stage.enabled system_setting toggle (§87 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "japa_stage.enabled", false)
}

// GetByUsername returns the public japa stage profile for a username. If the
// profile is not public (japa_stage_is_public = false), an empty DTO is
// returned without error so the public view simply shows nothing.
func (s *Service) GetByUsername(ctx context.Context, tenantID pgtype.UUID, username string) (PublicProfileDTO, error) {
	row, err := s.store.GetProfileByUsername(ctx, username, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return PublicProfileDTO{}, pgx.ErrNoRows
		}
		return PublicProfileDTO{}, fmt.Errorf("japastage: get by username: %w", err)
	}
	if !row.JapaStagePublic {
		return PublicProfileDTO{}, nil
	}
	return profileRowToPublicDTO(row), nil
}

// UpdateOwn updates the authenticated user's own japa stage and destination
// space. The stage must be one of the allowed values; destSpaceID may be
// empty to clear it.
func (s *Service) UpdateOwn(ctx context.Context, userID, tenantID pgtype.UUID, input UpdateStageInput) (ProfileDTO, error) {
	if !ValidStage(input.Stage) {
		return ProfileDTO{}, fmt.Errorf("invalid stage %q; must be one of: %s", input.Stage, JoinStages())
	}

	var destSpaceID pgtype.UUID
	if input.DestSpaceID != "" {
		if err := destSpaceID.Scan(input.DestSpaceID); err != nil {
			return ProfileDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
		}
	}

	if err := s.store.UpdateStage(ctx, userID, tenantID, input.Stage, destSpaceID); err != nil {
		if err == pgx.ErrNoRows {
			return ProfileDTO{}, pgx.ErrNoRows
		}
		return ProfileDTO{}, fmt.Errorf("japastage: update own: %w", err)
	}

	row, err := s.store.GetUserProfile(ctx, userID, tenantID)
	if err != nil {
		return ProfileDTO{}, fmt.Errorf("japastage: reload profile: %w", err)
	}
	return profileRowToDTO(row), nil
}

// SetPublic toggles the japa_stage_is_public visibility flag for the
// authenticated user.
func (s *Service) SetPublic(ctx context.Context, userID, tenantID pgtype.UUID, isPublic bool) (ProfileDTO, error) {
	if err := s.store.SetPublic(ctx, userID, tenantID, isPublic); err != nil {
		if err == pgx.ErrNoRows {
			return ProfileDTO{}, pgx.ErrNoRows
		}
		return ProfileDTO{}, fmt.Errorf("japastage: set public: %w", err)
	}

	row, err := s.store.GetUserProfile(ctx, userID, tenantID)
	if err != nil {
		return ProfileDTO{}, fmt.Errorf("japastage: reload profile: %w", err)
	}
	return profileRowToDTO(row), nil
}

// ListByStage returns a paginated directory of public profiles for a given
// stage. Only profiles with japa_stage_is_public = true are included.
func (s *Service) ListByStage(ctx context.Context, tenantID pgtype.UUID, stage string, page, limit int32) ([]PublicProfileDTO, int64, error) {
	if !ValidStage(stage) {
		return nil, 0, fmt.Errorf("invalid stage %q; must be one of: %s", stage, JoinStages())
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.ListByStage(ctx, tenantID, stage, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("japastage: list by stage: %w", err)
	}

	dtos := make([]PublicProfileDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, directoryRowToDTO(r))
	}
	return dtos, total, nil
}

// profileRowToDTO converts a ProfileRow (joined query result) to a ProfileDTO
// (own view — includes history and visibility).
func profileRowToDTO(r ProfileRow) ProfileDTO {
	dto := ProfileDTO{
		UserID:    r.UserID.String(),
		IsPublic:  r.JapaStagePublic,
		UpdatedAt: formatTs(r.JapaStageUpdated),
	}
	if r.Username != nil {
		dto.Username = *r.Username
	}
	if r.DisplayName != nil {
		dto.DisplayName = *r.DisplayName
	}
	if r.AvatarURL != nil {
		dto.AvatarURL = *r.AvatarURL
	}
	if r.JapaStage != nil {
		dto.Stage = *r.JapaStage
	}
	if r.JapaStageDestID.Valid {
		dto.DestSpaceID = r.JapaStageDestID.String()
	}
	if r.JapaStageDestName != nil {
		dto.DestSpaceName = *r.JapaStageDestName
	}
	if r.JapaStageDestCode != nil {
		dto.DestSpaceCode = *r.JapaStageDestCode
	}
	if len(r.JapaStageHistory) > 0 && string(r.JapaStageHistory) != "null" {
		dto.History = json.RawMessage(r.JapaStageHistory)
	}
	return dto
}

// profileRowToPublicDTO converts a ProfileRow to a PublicProfileDTO (public
// view — omits history and visibility).
func profileRowToPublicDTO(r ProfileRow) PublicProfileDTO {
	dto := PublicProfileDTO{
		UserID:    r.UserID.String(),
		UpdatedAt: formatTs(r.JapaStageUpdated),
	}
	if r.Username != nil {
		dto.Username = *r.Username
	}
	if r.DisplayName != nil {
		dto.DisplayName = *r.DisplayName
	}
	if r.AvatarURL != nil {
		dto.AvatarURL = *r.AvatarURL
	}
	if r.JapaStage != nil {
		dto.Stage = *r.JapaStage
	}
	if r.JapaStageDestID.Valid {
		dto.DestSpaceID = r.JapaStageDestID.String()
	}
	if r.JapaStageDestName != nil {
		dto.DestSpaceName = *r.JapaStageDestName
	}
	if r.JapaStageDestCode != nil {
		dto.DestSpaceCode = *r.JapaStageDestCode
	}
	return dto
}

// directoryRowToDTO converts a DirectoryRow to a PublicProfileDTO.
func directoryRowToDTO(r DirectoryRow) PublicProfileDTO {
	dto := PublicProfileDTO{
		UserID:    r.UserID.String(),
		UpdatedAt: formatTs(r.JapaStageUpdated),
	}
	if r.Username != nil {
		dto.Username = *r.Username
	}
	if r.DisplayName != nil {
		dto.DisplayName = *r.DisplayName
	}
	if r.AvatarURL != nil {
		dto.AvatarURL = *r.AvatarURL
	}
	if r.JapaStage != nil {
		dto.Stage = *r.JapaStage
	}
	if r.JapaStageDestID.Valid {
		dto.DestSpaceID = r.JapaStageDestID.String()
	}
	if r.JapaStageDestName != nil {
		dto.DestSpaceName = *r.JapaStageDestName
	}
	if r.JapaStageDestCode != nil {
		dto.DestSpaceCode = *r.JapaStageDestCode
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


