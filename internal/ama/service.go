// Package ama — service.go orchestrates AMA thread listing, retrieval,
// and closing. Feature-flagged via forum.ama.enabled system_setting (§88).
//
// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
//      japaconnect-engineering/adr/ADR-096-ask-an-expat.md
package ama

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// AMAThreadDTO is the API response shape for an AMA thread.
type AMAThreadDTO struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Slug         string          `json:"slug"`
	Body         string          `json:"body"`
	Status       string          `json:"status"`
	AuthorUserID string          `json:"author_user_id,omitempty"`
	AuthorName   string          `json:"author_name,omitempty"`
	AuthorAvatar string          `json:"author_avatar,omitempty"`
	AmaConfig    json.RawMessage `json:"ama_config"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

// Service holds the business logic for AMA threads.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates an AMA service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// Enabled reads the forum.ama.enabled system_setting toggle.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "forum.ama.enabled", false)
}

// CooldownDays returns the AMA cooldown period in days.
func (s *Service) CooldownDays(ctx context.Context, tenantID pgtype.UUID) int {
	if s.settings == nil {
		return 30
	}
	return s.settings.GetIntOr(ctx, tenantID, "ama_cooldown_days", 30)
}

// ListActive returns active AMA sessions (not yet closed).
func (s *Service) ListActive(ctx context.Context, tenantID pgtype.UUID, page, limit int32) ([]AMAThreadDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	threads, total, err := s.store.ListActive(ctx, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("ama: list active: %w", err)
	}

	dtos := make([]AMAThreadDTO, 0, len(threads))
	for _, t := range threads {
		dtos = append(dtos, threadToDTO(t))
	}
	return dtos, total, nil
}

// GetByID returns a single AMA thread by ID.
func (s *Service) GetByID(ctx context.Context, id, tenantID pgtype.UUID) (AMAThreadDTO, error) {
	thread, err := s.store.GetByID(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AMAThreadDTO{}, pgx.ErrNoRows
		}
		return AMAThreadDTO{}, fmt.Errorf("ama: get: %w", err)
	}
	return threadToDTO(thread), nil
}

// CloseAMA closes an AMA session (host or moderator only).
func (s *Service) CloseAMA(ctx context.Context, threadID, tenantID pgtype.UUID) error {
	if err := s.store.CloseAMA(ctx, threadID, tenantID); err != nil {
		return fmt.Errorf("ama: close: %w", err)
	}
	return nil
}

// threadToDTO converts an AMAThread to an AMAThreadDTO.
func threadToDTO(t AMAThread) AMAThreadDTO {
	dto := AMAThreadDTO{
		ID:           t.ID.String(),
		Title:        t.Title,
		Slug:         t.Slug,
		Body:         t.Body,
		Status:       t.Status,
		AuthorUserID: t.AuthorID.String(),
		AmaConfig:    json.RawMessage(t.AmaConfig),
		CreatedAt:    formatTs(t.CreatedAt),
		UpdatedAt:    formatTs(t.UpdatedAt),
	}
	if t.AuthorName != nil {
		dto.AuthorName = *t.AuthorName
	}
	if t.AuthorAvatar != nil {
		dto.AuthorAvatar = *t.AuthorAvatar
	}
	return dto
}
