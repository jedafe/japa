// Package calendar — service.go orchestrates calendar entry browsing,
// submission, save/subscribe, and admin review. Feature-flagged via
// calendar.enabled system_setting (§86).
//
// Ref: japaconnect-engineering/sections/86-community-calendar.md
//      japaconnect-engineering/adr/ADR-094-community-calendar.md
package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// CalendarEntryDTO is the API response shape for a calendar entry (list).
type CalendarEntryDTO struct {
	ID                 string `json:"id"`
	Title              string `json:"title"`
	Description        string `json:"description,omitempty"`
	Category           string `json:"category"`
	DestinationSpaceID string `json:"destination_space_id,omitempty"`
	OriginSpaceID      string `json:"origin_space_id,omitempty"`
	OccursAt           string `json:"occurs_at"`
	EndsAt             string `json:"ends_at,omitempty"`
	IsRecurring        bool   `json:"is_recurring"`
	RecurrencePattern  string `json:"recurrence_pattern,omitempty"`
	SourceURL          string `json:"source_url,omitempty"`
	Status             string `json:"status,omitempty"`
	SubmittedBy        string `json:"submitted_by,omitempty"`
	ReviewedBy         string `json:"reviewed_by,omitempty"`
	ReviewedAt         string `json:"reviewed_at,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

// SubmitEntryInput is the request body for POST /calendar.
type SubmitEntryInput struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	Category           string `json:"category"`
	DestinationSpaceID string `json:"destination_space_id"`
	OriginSpaceID      string `json:"origin_space_id"`
	OccursAt           string `json:"occurs_at"`
	EndsAt             string `json:"ends_at"`
	IsRecurring        bool   `json:"is_recurring"`
	RecurrencePattern  string `json:"recurrence_pattern"`
	SourceURL          string `json:"source_url"`
}

// SubscribeInput is the request body for POST /calendar/subscribe.
type SubscribeInput struct {
	DestinationSpaceID string `json:"destination_space_id"`
	Category           string `json:"category"`
}

// AdminUpdateEntryInput is the request body for PATCH /admin/calendar/:id.
type AdminUpdateEntryInput struct {
	Status string `json:"status"`
}

// Service holds the business logic for the Community Calendar.
type Service struct {
	store    *Store
	settings *config.Settings
}

// NewService creates a calendar service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// Enabled reads the calendar.enabled system_setting toggle.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "calendar.enabled", false)
}

// List returns approved calendar entries matching the given filters.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID pgtype.UUID, category string, from, to pgtype.Timestamptz, page, limit int32) ([]CalendarEntryDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	entries, total, err := s.store.ListEntries(ctx, ListEntriesFilter{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		Category:           category,
		From:               from,
		To:                 to,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("calendar: list: %w", err)
	}

	dtos := make([]CalendarEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, entryToDTO(e))
	}
	return dtos, total, nil
}

// Get returns a single approved calendar entry by id.
func (s *Service) Get(ctx context.Context, id, tenantID pgtype.UUID) (CalendarEntryDTO, error) {
	entry, err := s.store.GetEntry(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return CalendarEntryDTO{}, pgx.ErrNoRows
		}
		return CalendarEntryDTO{}, fmt.Errorf("calendar: get: %w", err)
	}
	return entryToDTO(entry), nil
}

// Submit creates a new pending calendar entry. The caller must have the
// calendar.submit permission (checked by the handler before calling).
func (s *Service) Submit(ctx context.Context, tenantID, userID pgtype.UUID, input SubmitEntryInput) (string, error) {
	if input.Title == "" {
		return "", fmt.Errorf("title is required")
	}
	if input.Category == "" {
		return "", fmt.Errorf("category is required")
	}
	if input.OccursAt == "" {
		return "", fmt.Errorf("occurs_at is required")
	}

	occursAt, err := parseTimestamp(input.OccursAt)
	if err != nil {
		return "", fmt.Errorf("invalid occurs_at: %w", err)
	}

	var endsAt pgtype.Timestamptz
	if input.EndsAt != "" {
		endsAt, err = parseTimestamp(input.EndsAt)
		if err != nil {
			return "", fmt.Errorf("invalid ends_at: %w", err)
		}
	}

	var destSpaceID pgtype.UUID
	if input.DestinationSpaceID != "" {
		if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
			return "", fmt.Errorf("invalid destination_space_id: %w", err)
		}
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return "", fmt.Errorf("invalid origin_space_id: %w", err)
		}
	}

	entry := CalendarEntry{
		TenantID:           tenantID,
		Title:              input.Title,
		Category:           input.Category,
		DestinationSpaceID: destSpaceID,
		OriginSpaceID:      originSpaceID,
		OccursAt:           occursAt,
		EndsAt:             endsAt,
		IsRecurring:        input.IsRecurring,
		SubmittedBy:        userID,
	}

	if input.Description != "" {
		entry.Description = &input.Description
	}
	if input.RecurrencePattern != "" {
		entry.RecurrencePattern = &input.RecurrencePattern
	}
	if input.SourceURL != "" {
		entry.SourceURL = &input.SourceURL
	}

	entryID, err := s.store.CreateEntry(ctx, entry)
	if err != nil {
		return "", fmt.Errorf("calendar: submit: %w", err)
	}
	return entryID.String(), nil
}

// Save bookmarks a calendar entry for the given user.
func (s *Service) Save(ctx context.Context, userID, entryID, tenantID pgtype.UUID) error {
	if err := s.store.SaveEntry(ctx, userID, entryID, tenantID); err != nil {
		return fmt.Errorf("calendar: save: %w", err)
	}
	return nil
}

// Unsave removes a bookmarked calendar entry for the given user.
func (s *Service) Unsave(ctx context.Context, userID, entryID, tenantID pgtype.UUID) error {
	if err := s.store.UnsaveEntry(ctx, userID, entryID, tenantID); err != nil {
		return fmt.Errorf("calendar: unsave: %w", err)
	}
	return nil
}

// Subscribe creates a category/destination subscription for the given user.
func (s *Service) Subscribe(ctx context.Context, userID, tenantID, destSpaceID pgtype.UUID, category string) error {
	if err := s.store.Subscribe(ctx, userID, tenantID, destSpaceID, category); err != nil {
		return fmt.Errorf("calendar: subscribe: %w", err)
	}
	return nil
}

// Unsubscribe removes a category/destination subscription for the given user.
func (s *Service) Unsubscribe(ctx context.Context, userID, tenantID, destSpaceID pgtype.UUID, category string) error {
	if err := s.store.Unsubscribe(ctx, userID, tenantID, destSpaceID, category); err != nil {
		return fmt.Errorf("calendar: unsubscribe: %w", err)
	}
	return nil
}

// AdminList returns all calendar entries (any status) for admin management.
func (s *Service) AdminList(ctx context.Context, tenantID, destSpaceID pgtype.UUID, category, status, q string, from, to pgtype.Timestamptz, page, limit int32) ([]CalendarEntryDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	entries, total, err := s.store.AdminListEntries(ctx, AdminListEntriesFilter{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		Category:           category,
		Status:             status,
		From:               from,
		To:                 to,
		Q:                  q,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("calendar: admin list: %w", err)
	}

	dtos := make([]CalendarEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, entryToDTO(e))
	}
	return dtos, total, nil
}

// AdminUpdate updates the status of a calendar entry (moderator+).
// Valid statuses: approved, archived, flagged.
func (s *Service) AdminUpdate(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, status string) error {
	switch status {
	case "approved", "archived", "flagged":
	default:
		return fmt.Errorf("invalid status: %s", status)
	}

	if err := s.store.AdminUpdateEntry(ctx, id, tenantID, reviewerID, status); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("calendar: admin update: %w", err)
	}
	return nil
}

// entryToDTO converts a CalendarEntry to a CalendarEntryDTO.
func entryToDTO(e CalendarEntry) CalendarEntryDTO {
	dto := CalendarEntryDTO{
		ID:          e.ID.String(),
		Title:       e.Title,
		Category:    e.Category,
		IsRecurring: e.IsRecurring,
		Status:      e.Status,
		OccursAt:    formatTs(e.OccursAt),
		CreatedAt:   formatTs(e.CreatedAt),
	}
	if e.Description != nil {
		dto.Description = *e.Description
	}
	if e.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = e.DestinationSpaceID.String()
	}
	if e.OriginSpaceID.Valid {
		dto.OriginSpaceID = e.OriginSpaceID.String()
	}
	if e.RecurrencePattern != nil {
		dto.RecurrencePattern = *e.RecurrencePattern
	}
	if e.SourceURL != nil {
		dto.SourceURL = *e.SourceURL
	}
	if e.SubmittedBy.Valid {
		dto.SubmittedBy = e.SubmittedBy.String()
	}
	if e.ReviewedBy.Valid {
		dto.ReviewedBy = e.ReviewedBy.String()
	}
	dto.EndsAt = formatTs(e.EndsAt)
	dto.ReviewedAt = formatTs(e.ReviewedAt)
	dto.UpdatedAt = formatTs(e.UpdatedAt)
	return dto
}

// formatTs renders a pgtype.Timestamptz as RFC 3339 UTC, or "" if NULL.
func formatTs(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// parseTimestamp parses an RFC 3339 string into a pgtype.Timestamptz.
func parseTimestamp(s string) (pgtype.Timestamptz, error) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return pgtype.Timestamptz{}, err
	}
	return pgtype.Timestamptz{Time: t, Valid: true}, nil
}
