// Package calendar — service.go orchestrates calendar entry browsing,
// submission, save/subscribe, and admin review. Feature-flagged via
// calendar.enabled system_setting (§86).
// // Ref: japaconnect-engineering/sections/86-community-calendar.md
// //	japaconnect-engineering/adr/ADR-094-community-calendar.md
package calendar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/ai"
	"github.com/jedafe/japa/internal/config"
)

// Notifier is the interface used by the calendar service to dispatch
// reminder notifications. It is satisfied by *notifications.Service.
// Defined here (rather than importing notifications directly) to avoid
// an import cycle: notifications → worker → calendar.
// Ref: japaconnect-engineering/sections/35-notification-system.md.
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// CalendarEntryDTO is the API response shape for a calendar entry (list).
// //nolint:revive // exported: stutter naming is intentional
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
	// ADR-088 universal AI content generation columns.
	AiGenerated     bool   `json:"ai_generated"`
	AiModel         string `json:"ai_model,omitempty"`
	HitlReviewedBy  string `json:"hitl_reviewed_by,omitempty"`
	AiGeneratedAt   string `json:"ai_generated_at,omitempty"`
	AiPromptHash    string `json:"ai_prompt_hash,omitempty"`
	HitlReviewedAt  string `json:"hitl_reviewed_at,omitempty"`
	HitlReviewNotes string `json:"hitl_review_notes,omitempty"`
	IsSaved         bool   `json:"is_saved,omitempty"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at,omitempty"`
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

// GenerateDraftRequest is the input for AI-generated calendar entry drafts
// (ADR-088, routed from the Content Studio).
type GenerateDraftRequest struct {
	DestinationSpaceID string `json:"destination_space_id"`
	Category           string `json:"category"`
	Prompt             string `json:"prompt"`
}

// GenerateDraftResponse is the response from Content Agent draft generation.
type GenerateDraftResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	EntryID string `json:"entry_id"`
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

// UpdateEntryInput is the request body for PATCH /calendar/:id.
// It mirrors SubmitEntryInput but omits immutable ownership/admin fields.
type UpdateEntryInput SubmitEntryInput

// AIDispatcher is the subset of the AI Gateway the calendar service needs
// to generate draft entries (content_generation category).
type AIDispatcher interface {
	Dispatch(ctx context.Context, category string, req ai.Request) (ai.Response, error)
}

// ErrHITLReviewRequired is returned by AdminUpdate when an AI-generated
// entry is being published without prior HITL review (ADR-088 gate).
var ErrHITLReviewRequired = errors.New("calendar: HITL review required before publishing AI-generated content")

// ErrSourceURLRequired is returned by AdminUpdate when an AI-generated
// entry with a placeholder source_url is being published (§86 requires a
// verifiable official source link).
var ErrSourceURLRequired = errors.New("calendar: a verifiable official source_url is required before publishing")

// ErrEditForbidden is returned when a user attempts to edit a calendar entry
// they do not own and they do not hold calendar.edit_any.
var ErrEditForbidden = errors.New("calendar: edit forbidden")

// permissionChecker is the subset of rbac.RBAC used for edit authorization.
type permissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// Service holds the business logic for the Community Calendar.
type Service struct {
	store      *Store
	settings   *config.Settings
	notifier   Notifier
	dispatcher AIDispatcher
	rbac       permissionChecker
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

// SetDispatcher injects the AI Gateway dispatcher for draft generation.
func (s *Service) SetDispatcher(d AIDispatcher) { s.dispatcher = d }

// SetRBAC injects the permission resolver used for edit authorization
// (calendar.edit_own / calendar.edit_any).
func (s *Service) SetRBAC(r permissionChecker) { s.rbac = r }

// Enabled reads the calendar.enabled system_setting toggle.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "calendar.enabled", false)
}

// CategoryOption is a single allowed calendar category (§86).
// BUG-009: categories are served from the calendar.categories
// system_setting (jsonb array of {value, label}) so they are
// admin-configurable and consumed via the API (Rule 3, Rule 16).
type CategoryOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// defaultCategories is the fallback when the system_setting is not set.
var defaultCategories = []CategoryOption{
	{Value: "visa_window", Label: "Visa window"},
	{Value: "embassy_closure", Label: "Embassy closure"},
	{Value: "application_deadline", Label: "Application deadline"},
	{Value: "exam_window", Label: "Exam window"},
	{Value: "immigration_office", Label: "Immigration office"},
	{Value: "other", Label: "Other"},
}

// Categories returns the allowed calendar categories from the
// calendar.categories system_setting, falling back to the default list.
// The fallback also covers the case where the settings store is not
// fully initialised (e.g. in unit tests with a zero-value Settings).
func (s *Service) Categories(ctx context.Context) []CategoryOption {
	if s.settings == nil {
		return defaultCategories
	}

	raw, err := s.safeGetSetting(ctx, "calendar.categories")
	if err != nil || raw == "" {
		return defaultCategories
	}

	var cats []CategoryOption
	if err := json.Unmarshal([]byte(raw), &cats); err != nil || len(cats) == 0 {
		return defaultCategories
	}

	return cats
}

// safeGetSetting wraps Settings.GetSetting with a panic recover so that
// a zero-value Settings (no DB pool) returns an error instead of crashing.
// This keeps unit tests that construct NewService(nil, &config.Settings{})
// working without a full DB harness.
func (s *Service) safeGetSetting(ctx context.Context, key string) (val string, err error) {
	defer func() {
		if r := recover(); r != nil {
			val = ""
			err = fmt.Errorf("settings: panic: %v", r)
		}
	}()

	return s.settings.GetSetting(ctx, key)
}

// ValidCategory checks whether a category value is in the allowed list.
func (s *Service) ValidCategory(ctx context.Context, category string) bool {
	for _, c := range s.Categories(ctx) {
		if c.Value == category {
			return true
		}
	}

	return false
}

// List returns published calendar entries matching the given filters.
func (s *Service) List(ctx context.Context, destSpaceID, originSpaceID pgtype.UUID, category string, from, to pgtype.Timestamptz, page, limit int32, userID pgtype.UUID) ([]CalendarEntryDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	entries, total, err := s.store.ListEntries(ctx, ListEntriesFilter{

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
		dto := entryToDTO(e)
		if userID.Valid {
			saved, err := s.store.IsSaved(ctx, userID, e.ID)
			if err != nil {
				slog.Warn("calendar: list: failed to check is_saved", "entry_id", e.ID, "error", err)
			}
			dto.IsSaved = saved
		}
		dtos = append(dtos, dto)
	}

	return dtos, total, nil
}

// Get returns a single approved calendar entry by id.
func (s *Service) Get(ctx context.Context, id, userID pgtype.UUID) (CalendarEntryDTO, error) {
	entry, err := s.store.GetEntry(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CalendarEntryDTO{}, pgx.ErrNoRows
		}

		return CalendarEntryDTO{}, fmt.Errorf("calendar: get: %w", err)
	}

	dto := entryToDTO(entry)
	if userID.Valid {
		saved, err := s.store.IsSaved(ctx, userID, entry.ID)
		if err != nil {
			slog.Warn("calendar: get: failed to check is_saved", "entry_id", entry.ID, "error", err)
		}
		dto.IsSaved = saved
	}

	return dto, nil
}

// Update edits a calendar entry's content fields. Ownership is enforced unless
// the caller holds calendar.edit_any. Status, moderation and AI provenance
// metadata are not modified by this endpoint.
func (s *Service) Update(ctx context.Context, id, tenantID, callerID pgtype.UUID, input UpdateEntryInput) (CalendarEntryDTO, error) {
	existing, err := s.store.GetEntryForUpdate(ctx, id, tenantID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CalendarEntryDTO{}, pgx.ErrNoRows
		}

		return CalendarEntryDTO{}, fmt.Errorf("calendar: update: %w", err)
	}

	editAny := s.rbac != nil && s.rbac.HasPermission(ctx, callerID, "calendar.edit_any")
	if !editAny && (!existing.SubmittedBy.Valid || existing.SubmittedBy != callerID) {
		return CalendarEntryDTO{}, ErrEditForbidden
	}

	entry, err := s.buildEntry(ctx, SubmitEntryInput(input))
	if err != nil {
		return CalendarEntryDTO{}, err
	}

	updated, err := s.store.UpdateEntry(ctx, id, tenantID, entry)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CalendarEntryDTO{}, pgx.ErrNoRows
		}

		return CalendarEntryDTO{}, fmt.Errorf("calendar: update: %w", err)
	}

	return entryToDTO(updated), nil
}

// buildEntry validates a submit/update input and converts it into a
// CalendarEntry value. The caller is responsible for setting Status,
// SubmittedBy and ID (for update) as appropriate.
func (s *Service) buildEntry(ctx context.Context, input SubmitEntryInput) (CalendarEntry, error) {
	if input.Title == "" {
		return CalendarEntry{}, errors.New("title is required")
	}

	if input.Category == "" {
		return CalendarEntry{}, errors.New("category is required")
	}
	// BUG-009: validate category against the dynamic calendar.categories
	// system_setting list (Rule 3: no hardcoded config).
	if !s.ValidCategory(ctx, input.Category) {
		return CalendarEntry{}, fmt.Errorf("invalid category: %s", input.Category)
	}

	if input.OccursAt == "" {
		return CalendarEntry{}, errors.New("occurs_at is required")
	}
	// §86: "Every entry must include a verifiable official source link."
	if input.SourceURL == "" {
		return CalendarEntry{}, errors.New("source_url is required")
	}
	// BUG-011: validate source_url scheme is http or https to prevent
	// stored XSS via javascript:/data: URIs rendered as <a href> in the
	// frontend. Ref: japaconnect-engineering/sections/5.1-xss-prevention.md
	parsedURL, err := url.Parse(input.SourceURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return CalendarEntry{}, errors.New("source_url must be a valid http or https URL")
	}

	occursAt, err := parseTimestamp(input.OccursAt)
	if err != nil {
		return CalendarEntry{}, fmt.Errorf("invalid occurs_at: %w", err)
	}

	var endsAt pgtype.Timestamptz
	if input.EndsAt != "" {
		endsAt, err = parseTimestamp(input.EndsAt)
		if err != nil {
			return CalendarEntry{}, fmt.Errorf("invalid ends_at: %w", err)
		}
	}

	var destSpaceID pgtype.UUID
	if input.DestinationSpaceID != "" {
		if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
			return CalendarEntry{}, fmt.Errorf("invalid destination_space_id: %w", err)
		}
	}

	var originSpaceID pgtype.UUID
	if input.OriginSpaceID != "" {
		if err := originSpaceID.Scan(input.OriginSpaceID); err != nil {
			return CalendarEntry{}, fmt.Errorf("invalid origin_space_id: %w", err)
		}
	}

	entry := CalendarEntry{

		Title:              input.Title,
		Category:           input.Category,
		DestinationSpaceID: destSpaceID,
		OriginSpaceID:      originSpaceID,
		OccursAt:           occursAt,
		EndsAt:             endsAt,
		IsRecurring:        input.IsRecurring,
		SourceURL:          input.SourceURL,
	}

	if input.Description != "" {
		entry.Description = &input.Description
	}

	if input.RecurrencePattern != "" {
		entry.RecurrencePattern = &input.RecurrencePattern
	}

	// BUG-002: compute staleness_check_due BEFORE the write. For recurring
	// entries the staleness check is due 60 days before the occurrence (§86
	// Staleness Management). Non-recurring entries get a NULL value.
	if input.IsRecurring {
		entry.StalenessCheckDue = pgtype.Timestamptz{
			Time:  occursAt.Time.AddDate(0, 0, -60),
			Valid: true,
		}
	}

	return entry, nil
}

// Submit creates a new pending calendar entry. The caller must have the
// calendar.submit permission (checked by the handler before calling).
func (s *Service) Submit(ctx context.Context, userID pgtype.UUID, input SubmitEntryInput) (string, error) {
	entry, err := s.buildEntry(ctx, input)
	if err != nil {
		return "", err
	}

	entry.Status = "pending"
	entry.SubmittedBy = userID

	entryID, err := s.store.CreateEntry(ctx, entry)
	if err != nil {
		return "", fmt.Errorf("calendar: submit: %w", err)
	}

	return entryID.String(), nil
}

// Save bookmarks a calendar entry for the given user.
func (s *Service) Save(ctx context.Context, userID, entryID pgtype.UUID) error {
	if err := s.store.SaveEntry(ctx, userID, entryID); err != nil {
		return fmt.Errorf("calendar: save: %w", err)
	}

	return nil
}

// Unsave removes a bookmarked calendar entry for the given user.
func (s *Service) Unsave(ctx context.Context, userID, entryID pgtype.UUID) error {
	if err := s.store.UnsaveEntry(ctx, userID, entryID); err != nil {
		return fmt.Errorf("calendar: unsave: %w", err)
	}

	return nil
}

// ListSaved returns the calendar entries saved by the given user.
// Used by GET /me/calendar/saved (§86 Integration: "Saved entries appear on
// the user dashboard alongside visa checklists.").
func (s *Service) ListSaved(ctx context.Context, userID pgtype.UUID, page, limit int) ([]CalendarEntryDTO, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	if page < 1 {
		page = 1
	}

	offset := (page - 1) * limit

	entries, total, err := s.store.ListSavedEntries(ctx, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("calendar: list saved: %w", err)
	}

	dtos := make([]CalendarEntryDTO, 0, len(entries))
	for _, e := range entries {
		dto := entryToDTO(e)
		dto.IsSaved = true
		dtos = append(dtos, dto)
	}

	return dtos, total, nil
}

// Subscribe creates a category/destination subscription for the given user.
func (s *Service) Subscribe(ctx context.Context, userID, destSpaceID pgtype.UUID, category string) error {
	if err := s.store.Subscribe(ctx, userID, destSpaceID, category); err != nil {
		return fmt.Errorf("calendar: subscribe: %w", err)
	}

	return nil
}

// Unsubscribe removes a category/destination subscription for the given user.
func (s *Service) Unsubscribe(ctx context.Context, userID, destSpaceID pgtype.UUID, category string) error {
	if err := s.store.Unsubscribe(ctx, userID, destSpaceID, category); err != nil {
		return fmt.Errorf("calendar: unsubscribe: %w", err)
	}

	return nil
}

// AdminList returns all calendar entries (any status) for admin management.
func (s *Service) AdminList(ctx context.Context, destSpaceID, originSpaceID pgtype.UUID, category, status, q string, from, to pgtype.Timestamptz, sort, order string, page, limit int32) ([]CalendarEntryDTO, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	entries, total, err := s.store.AdminListEntries(ctx, AdminListEntriesFilter{

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
// BUG-003 / ADR-088: AI-generated entries must pass HITL review before
// publication, and §86 requires a verifiable source_url for every published
// entry. The reason, when provided, is persisted as hitl_review_notes.
func (s *Service) AdminUpdate(ctx context.Context, id, reviewerID pgtype.UUID, status, reason string) error {
	switch status {
	case "published", "archived", "unverified":
	default:
		return fmt.Errorf("invalid status: %s", status)
	}

	row, err := s.store.GetEntryForReview(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("calendar: admin update: %w", err)
	}

	if status == "published" {
		if row.SourceUrl == "" {
			return ErrSourceURLRequired
		}

		if row.AiGenerated && !row.HitlReviewedBy.Valid {
			if reason == "" {
				return ErrHITLReviewRequired
			}
		}
	}

	if err := s.store.AdminUpdateEntry(ctx, id, reviewerID, status, reason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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

// AdminSoftDelete soft-deletes a calendar entry (admin+). Per Rule 11, user
// content is soft-deleted, never hard-deleted. Gated by calendar.manage.
// Ref: japaconnect-engineering/sections/86-community-calendar.md
// Ref: japaconnect-engineering/AGENTS.md Rule 11.
func (s *Service) AdminSoftDelete(ctx context.Context, id pgtype.UUID) error {
	if err := s.store.AdminSoftDeleteEntry(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("calendar: admin soft delete: %w", err)
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
// It finds published entries occurring at the 90/30/7-day marks and
// dispatches reminder notifications to subscribers per §86, ADR-094.
// Uses the notification system (§35) to create in-app + email + push
// notifications based on user preferences.
// // BUG-012: the store now pre-filters in SQL to only entries at the
// 90/30/7-day marks, so the Go loop only processes entries that are
// actually due a reminder.
func (s *Service) SendReminders(ctx context.Context) (int, error) {
	// §86, ADR-094: reminders fire at 90/30/7-day marks.
	dayMarks := []int{90, 30, 7}

	entries, err := s.store.UpcomingEntriesForReminders(ctx, dayMarks)
	if err != nil {
		return 0, fmt.Errorf("calendar: send reminders: %w", err)
	}

	dispatched := 0

	for _, e := range entries {
		// Calculate whole days until the event for the notification body.
		now := time.Now().UTC()

		daysUntil := int(e.OccursAt.Time.UTC().Sub(now).Hours() / 24)
		if daysUntil < 0 {
			daysUntil = 0
		}

		// Query subscribers for this entry's destination+category.
		subscribers, err := s.store.SubscribersForEntry(ctx, e)
		if err != nil {
			slog.Warn("calendar: failed to get subscribers for reminder", "entry_id", e.ID.String(), "error", err)
			continue
		}

		if len(subscribers) == 0 {
			continue
		}

		title := "Upcoming event: " + e.Title
		body := fmt.Sprintf("A calendar event you subscribed to is happening in %d days (on %s).",
			daysUntil, e.OccursAt.Time.UTC().Format("2006-01-02"))
		actionURL := "/calendar/" + e.ID.String()

		for _, userID := range subscribers {
			if s.notifier == nil {
				break
			}

			if err := s.notifier.CreateNotification(
				ctx,

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
				slog.Warn("calendar: failed to dispatch reminder notification", "entry_id", e.ID.String(), "user_id", userID.String(), "error", err)
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
		// ADR-088 AI generation columns.
		AiGenerated: e.AiGenerated,
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

	dto.SourceURL = e.SourceURL
	if e.SubmittedBy.Valid {
		dto.SubmittedBy = e.SubmittedBy.String()
	}

	if e.ReviewedBy.Valid {
		dto.ReviewedBy = e.ReviewedBy.String()
	}

	if e.AiModel != nil {
		dto.AiModel = *e.AiModel
	}

	if e.HitlReviewedBy.Valid {
		dto.HitlReviewedBy = e.HitlReviewedBy.String()
	}

	if e.AiPromptHash != nil {
		dto.AiPromptHash = *e.AiPromptHash
	}

	if e.HitlReviewNotes != nil {
		dto.HitlReviewNotes = *e.HitlReviewNotes
	}

	dto.EndsAt = formatTs(e.EndsAt)
	dto.ReviewedAt = formatTs(e.ReviewedAt)
	dto.AiGeneratedAt = formatTs(e.AiGeneratedAt)
	dto.HitlReviewedAt = formatTs(e.HitlReviewedAt)
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

// GenerateDraft creates a calendar entry draft via the Content Agent (§86,
// ADR-088). The draft is inserted with status='pending', then stamped with
// ai_generated=true, ai_model='content_agent_v1'. It requires HITL approval
// before publication. Admin-only (community_calendar.manage permission).
// // When an AI dispatcher is wired (SetDispatcher), the method calls the AI
// Gateway via the "content_generation" category. If the dispatcher is nil,
// the draft is still created with the prompt as description so a reviewer
// can fill it manually (graceful degradation).
func (s *Service) GenerateDraft(ctx context.Context, tenantID pgtype.UUID, input GenerateDraftRequest) (GenerateDraftResponse, error) {
	if input.DestinationSpaceID == "" {
		return GenerateDraftResponse{}, errors.New("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	category := input.Category
	if category == "" || !s.ValidCategory(ctx, category) {
		category = "other"
	}

	// Build the prompt sent to the Content Agent.
	prompt := input.Prompt
	if prompt == "" {
		prompt = fmt.Sprintf(
			"Generate a community calendar entry for destination_space_id=%s, category=%s. Include title, description, date, and source URL.",
			input.DestinationSpaceID, category,
		)
	}

	// ai_prompt_hash = SHA256(prompt) for audit traceability.
	sum := sha256.Sum256([]byte(prompt))
	promptHash := hex.EncodeToString(sum[:])

	// Call the AI Gateway when a dispatcher is wired.
	description := prompt
	title := "AI Draft: " + category

	if s.dispatcher != nil {
		resp, aiErr := s.dispatcher.Dispatch(ctx, "content_generation", ai.Request{
			Category: "content_generation",
			Payload: map[string]any{
				"prompt": prompt,

				"guide_type": "calendar_entry",
			},
		})
		if aiErr != nil {
			return GenerateDraftResponse{}, fmt.Errorf("calendar: AI generation failed: %w", aiErr)
		}

		if text, ok := resp.Raw["text"].(string); ok {
			description = text
		} else if text, ok := resp.Raw["content"].(string); ok {
			description = text
		} else if text, ok := resp.Raw["output"].(string); ok {
			description = text
		}

		slog.Info("calendar: AI Gateway generated draft", "destination_space_id", input.DestinationSpaceID, "category", category, "prompt_hash", promptHash)
	}

	descPtr := &description
	// Create the calendar entry with status='pending'.
	// BUG-005: use an empty source_url for AI drafts — the reviewer must
	// fill in the real verifiable official source URL before publication.
	// The AdminUpdate HITL gate (BUG-003/BUG-005) blocks publishing AI
	// entries with an empty or placeholder source_url.
	entryID, err := s.store.CreateEntry(ctx, CalendarEntry{

		Title:              title,
		Description:        descPtr,
		Category:           category,
		DestinationSpaceID: destSpaceID,
		SourceURL:          "",
		Status:             "pending",
	})
	if err != nil {
		return GenerateDraftResponse{}, fmt.Errorf("calendar: generate draft: %w", err)
	}

	// Stamp the AI provenance columns (CreateEntry doesn't set them).
	// BUG-004: pass ai_prompt_hash for audit reproducibility (ADR-088).
	aiModel := "content_agent_v1"
	if err := s.store.MarkAIGenerated(ctx, entryID, aiModel, promptHash); err != nil {
		slog.Warn("calendar: mark ai generated failed, draft remains", "entry_id", entryID, "error", err)
	}

	return GenerateDraftResponse{
		Status:  "accepted",
		Message: "Content Agent draft created in 'pending' status. Review and approve to publish.",
		EntryID: entryID.String(),
	}, nil
}
