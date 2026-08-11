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
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
)

// Notifier is the interface used by the calendar service to dispatch
// reminder notifications. It is satisfied by *notifications.Service.
// Defined here (rather than importing notifications directly) to avoid
// an import cycle: notifications → worker → calendar.
// Ref: japaconnect-engineering/sections/35-notification-system.md
type Notifier interface {
	CreateNotification(ctx context.Context, tenantID, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

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
// Accepts both destination_space_id (canonical) and destination_country
// (frontend alias) for the destination field.
type SubscribeInput struct {
	DestinationSpaceID string `json:"destination_space_id"`
	DestinationCountry string `json:"destination_country"`
	Category           string `json:"category"`
}

// AdminUpdateEntryInput is the request body for PATCH /admin/calendar/:id.
type AdminUpdateEntryInput struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// Service holds the business logic for the Community Calendar.
type Service struct {
	store    *Store
	settings *config.Settings
	notifier Notifier
}

// NewService creates a calendar service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetNotifier injects the notification service used to dispatch calendar
// reminder notifications (§86, ADR-094). Called once at startup from
// server.go after the notification service is constructed.
func (s *Service) SetNotifier(n Notifier) {
	s.notifier = n
}

// Enabled reads the calendar.enabled system_setting toggle.
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "calendar.enabled", false)
}

// List returns published calendar entries matching the given filters.
func (s *Service) List(ctx context.Context, tenantID, destSpaceID, originSpaceID pgtype.UUID, category string, from, to pgtype.Timestamptz, page, limit int32) ([]CalendarEntryDTO, int64, error) {
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
		OriginSpaceID:      originSpaceID,
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
	// §86: "Every entry must include a verifiable official source link."
	if input.SourceURL == "" {
		return "", fmt.Errorf("source_url is required")
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
		SourceURL:          &input.SourceURL,
	}

	if input.Description != "" {
		entry.Description = &input.Description
	}
	if input.RecurrencePattern != "" {
		entry.RecurrencePattern = &input.RecurrencePattern
	}

	entryID, err := s.store.CreateEntry(ctx, entry)
	if err != nil {
		return "", fmt.Errorf("calendar: submit: %w", err)
	}

	// BUG-006: set staleness_check_due = occurs_at - 60 days for recurring
	// entries per §86 Staleness Management.
	if input.IsRecurring {
		if err := s.store.SetStalenessCheckDue(ctx, entryID); err != nil {
			slog.Warn("calendar: submit: failed to set staleness_check_due", "entry_id", entryID, "error", err)
		}
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
func (s *Service) AdminList(ctx context.Context, tenantID, destSpaceID, originSpaceID pgtype.UUID, category, status, q string, from, to pgtype.Timestamptz, sort, order string, page, limit int32) ([]CalendarEntryDTO, int64, error) {
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
		OriginSpaceID:      originSpaceID,
		Category:           category,
		Status:             status,
		From:               from,
		To:                 to,
		Q:                  q,
		Sort:               sort,
		Order:              order,
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
// Valid statuses: published, archived, unverified.
func (s *Service) AdminUpdate(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, status string) error {
	switch status {
	case "published", "archived", "unverified":
	default:
		return fmt.Errorf("invalid status: %s", status)
	}

	if err := s.store.AdminUpdateEntry(ctx, id, tenantID, reviewerID, status); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("calendar: admin update: %w", err)
	}

	// When an entry is re-verified (published), reset staleness_check_due for
	// recurring entries per §86 Staleness Management.
	if status == "published" {
		if err := s.store.SetStalenessCheckDue(ctx, id); err != nil {
			slog.Warn("calendar: admin update: failed to reset staleness_check_due", "entry_id", id, "error", err)
		}
	}
	return nil
}

// FlagStaleEntries is called by the calendar:staleness_check Asynq task.
func (s *Service) FlagStaleEntries(ctx context.Context) (int64, error) {
	n, err := s.store.FlagStaleEntries(ctx)
	if err != nil {
		return 0, fmt.Errorf("calendar: flag stale: %w", err)
	}
	return n, nil
}

// SendReminders is called by the calendar:send_reminders Asynq task.
// It finds published entries occurring within the next 90 days and
// dispatches reminder notifications to subscribers at the 90/30/7-day
// marks per §86, ADR-094. Uses the notification system (§35) to create
// in-app + email + push notifications based on user preferences.
func (s *Service) SendReminders(ctx context.Context) (int, error) {
	entries, err := s.store.UpcomingEntriesForReminders(ctx, 90)
	if err != nil {
		return 0, fmt.Errorf("calendar: send reminders: %w", err)
	}

	dispatched := 0
	for _, e := range entries {
		// Calculate whole days until the event. Use UTC midnight-to-midnight
		// to avoid firing at the wrong day due to time-of-day differences.
		now := time.Now().UTC()
		daysUntil := int(e.OccursAt.Time.UTC().Sub(now).Hours() / 24)
		if daysUntil < 0 {
			daysUntil = 0
		}

		// Only fire at the 90/30/7-day marks (§86, ADR-094).
		if daysUntil != 90 && daysUntil != 30 && daysUntil != 7 {
			continue
		}

		// Query subscribers for this entry's destination+category.
		subscribers, err := s.store.SubscribersForEntry(ctx, e)
		if err != nil {
			slog.Warn("calendar: failed to get subscribers for reminder",
				"entry_id", e.ID.String(), "error", err)
			continue
		}
		if len(subscribers) == 0 {
			continue
		}

		title := fmt.Sprintf("Upcoming event: %s", e.Title)
		body := fmt.Sprintf("A calendar event you subscribed to is happening in %d days (on %s).",
			daysUntil, e.OccursAt.Time.UTC().Format("2006-01-02"))
		actionURL := fmt.Sprintf("/calendar/%s", e.ID.String())

		for _, userID := range subscribers {
			if s.notifier == nil {
				break
			}
			if err := s.notifier.CreateNotification(
				ctx,
				e.TenantID,
				userID,
				"calendar_reminder",
				"system",
				pgtype.UUID{},
				"calendar_entry",
				e.ID,
				title,
				body,
				actionURL,
			); err != nil {
				slog.Warn("calendar: failed to dispatch reminder notification",
					"entry_id", e.ID.String(), "user_id", userID.String(), "error", err)
				continue
			}
			dispatched++
		}
	}

	return dispatched, nil
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
