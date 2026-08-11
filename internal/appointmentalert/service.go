// Package appointmentalert — service.go orchestrates monitored embassy
// configuration and per-user appointment alert CRUD. It validates input,
// enforces ownership (a user may only manage their own alerts), and
// converts Store rows into API DTOs. Feature-flagged via the
// appointment_alert.enabled system_setting (§91, ADR-099).
//
// Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md
//      japaconnect-engineering/adr/ADR-099-embassy-appointment-alerter.md
package appointmentalert

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/worker"
)

// EmbassyDTO is the API response shape for a monitored embassy.
type EmbassyDTO struct {
	ID                  string `json:"id"`
	DestinationSpaceID  string `json:"destination_space_id"`
	DestinationSpaceName string `json:"destination_space_name,omitempty"`
	Slug                string `json:"slug"`
	Name                string `json:"name"`
	BookingURL          string `json:"booking_url"`
	PollIntervalSeconds int32  `json:"poll_interval_seconds"`
	IsActive            bool   `json:"is_active"`
	LastPolledAt        string `json:"last_polled_at,omitempty"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

// AlertDTO is the API response shape for an appointment alert.
type AlertDTO struct {
	ID                   string   `json:"id"`
	DestinationSpaceID   string   `json:"destination_space_id"`
	DestinationSpaceName string   `json:"destination_space_name,omitempty"`
	EmbassySlug          string   `json:"embassy_slug"`
	VisaCategory         string   `json:"visa_category,omitempty"`
	DateRangeStart       string   `json:"date_range_start"`
	DateRangeEnd         string   `json:"date_range_end"`
	Status               string   `json:"status"`
	LastCheckedAt        string   `json:"last_checked_at,omitempty"`
	FiredAt              string   `json:"fired_at,omitempty"`
	ExpiresAt            string   `json:"expires_at,omitempty"`
	NotificationChannels []string `json:"notification_channels"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

// CreateEmbassyInput is the request body for POST /admin/embassies.
type CreateEmbassyInput struct {
	DestinationSpaceID  string `json:"destination_space_id"`
	Slug                string `json:"slug"`
	Name                string `json:"name"`
	BookingURL          string `json:"booking_url"`
	PollIntervalSeconds int32  `json:"poll_interval_seconds"`
	IsActive            *bool  `json:"is_active"`
}

// UpdateEmbassyInput is the request body for PATCH /admin/embassies/:id.
type UpdateEmbassyInput struct {
	Name                *string `json:"name"`
	BookingURL          *string `json:"booking_url"`
	PollIntervalSeconds *int32  `json:"poll_interval_seconds"`
	IsActive            *bool   `json:"is_active"`
}

// CreateAlertInput is the request body for POST /me/appointment-alerts.
type CreateAlertInput struct {
	DestinationSpaceID  string   `json:"destination_space_id"`
	EmbassySlug         string   `json:"embassy_slug"`
	VisaCategory        string   `json:"visa_category"`
	DateRangeStart      string   `json:"date_range_start"`
	DateRangeEnd        string   `json:"date_range_end"`
	ExpiresAt           string   `json:"expires_at"`
	NotificationChannels []string `json:"notification_channels"`
}

// UpdateAlertInput is the request body for PATCH /me/appointment-alerts/:id.
type UpdateAlertInput struct {
	Status               *string  `json:"status"`
	VisaCategory         *string  `json:"visa_category"`
	DateRangeStart       *string  `json:"date_range_start"`
	DateRangeEnd         *string  `json:"date_range_end"`
	ExpiresAt            *string  `json:"expires_at"`
	NotificationChannels []string `json:"notification_channels"`
}

// TaskEnqueuer is the minimal interface for enqueueing Asynq tasks.
// Implemented by *worker.Enqueuer; a nil enqueuer makes TriggerPoll a
// graceful no-op (development/tests).
type TaskEnqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service holds the business logic for the Embassy Appointment Alerter.
type Service struct {
	store    *Store
	settings *config.Settings
	enqueuer TaskEnqueuer
}

// NewService creates an appointment alerter service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetEnqueuer injects the Asynq task enqueuer for embassy poll/dispatch tasks.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// Enabled reads the appointment_alert.enabled system_setting toggle
// (§91 feature flag).
func (s *Service) Enabled(ctx context.Context, tenantID pgtype.UUID) bool {
	if s.settings == nil {
		return false
	}
	return s.settings.GetBoolOr(ctx, tenantID, "appointment_alert.enabled", false)
}

// ErrNotOwner is returned when a user tries to manage an alert they do not own.
var ErrNotOwner = fmt.Errorf("not the alert owner")

// ListEmbassies returns a paginated list of monitored embassies. When
// activeOnly is true, only embassies currently being polled are returned.
func (s *Service) ListEmbassies(ctx context.Context, tenantID, destSpaceID pgtype.UUID, activeOnly bool, page, limit int32) ([]EmbassyDTO, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.ListEmbassies(ctx, ListEmbassiesFilter{
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		ActiveOnly:         activeOnly,
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("appointmentalert: list embassies: %w", err)
	}

	dtos := make([]EmbassyDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, embassyToDTO(r))
	}
	return dtos, total, nil
}

// CreateEmbassy adds a new monitored embassy (admin only).
func (s *Service) CreateEmbassy(ctx context.Context, tenantID pgtype.UUID, input CreateEmbassyInput) (EmbassyDTO, error) {
	if input.Slug == "" {
		return EmbassyDTO{}, fmt.Errorf("slug is required")
	}
	if input.Name == "" {
		return EmbassyDTO{}, fmt.Errorf("name is required")
	}
	if input.BookingURL == "" {
		return EmbassyDTO{}, fmt.Errorf("booking_url is required")
	}
	if input.DestinationSpaceID == "" {
		return EmbassyDTO{}, fmt.Errorf("destination_space_id is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return EmbassyDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}
	pollInterval := input.PollIntervalSeconds
	if pollInterval <= 0 {
		pollInterval = 900
	}

	row, err := s.store.CreateEmbassy(ctx, EmbassyRow{
		TenantID:            tenantID,
		DestinationSpaceID:  destSpaceID,
		Slug:                input.Slug,
		Name:                input.Name,
		BookingURL:          input.BookingURL,
		PollIntervalSeconds: pollInterval,
		IsActive:            isActive,
	})
	if err != nil {
		return EmbassyDTO{}, fmt.Errorf("appointmentalert: create embassy: %w", err)
	}
	return embassyToDTO(row), nil
}

// UpdateEmbassy updates a monitored embassy's config (admin only).
func (s *Service) UpdateEmbassy(ctx context.Context, id, tenantID pgtype.UUID, input UpdateEmbassyInput) (EmbassyDTO, error) {
	row, err := s.store.UpdateEmbassy(ctx, id, tenantID, input.Name, input.BookingURL, input.PollIntervalSeconds, input.IsActive)
	if err != nil {
		if err == pgx.ErrNoRows {
			return EmbassyDTO{}, pgx.ErrNoRows
		}
		return EmbassyDTO{}, fmt.Errorf("appointmentalert: update embassy: %w", err)
	}
	return embassyToDTO(row), nil
}

// TriggerPoll marks a monitored embassy as polled (last_polled_at = now())
// and enqueues the embassy:poll Asynq task so the worker scrapes the booking
// page for open slots. The actual scraping is performed asynchronously by
// the Asynq worker (§91, ADR-099).
func (s *Service) TriggerPoll(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.MarkEmbassyPolled(ctx, id, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("appointmentalert: trigger poll: %w", err)
	}

	// Enqueue the poll task (fire-and-forget, Rule 8). A nil enqueuer
	// (development/tests) makes this a graceful no-op.
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"embassy_id": id.String(),
			"tenant_id":  tenantID.String(),
		})
		if err != nil {
			return fmt.Errorf("appointmentalert: trigger poll: marshal payload: %w", err)
		}
		if err := s.enqueuer.Enqueue(worker.TypeEmbassyPoll, payload); err != nil {
			return fmt.Errorf("appointmentalert: trigger poll: enqueue: %w", err)
		}
	}
	return nil
}

// ExpireAlerts transitions all active appointment_alerts past their
// expires_at (or date_range_end when expires_at is NULL) to 'expired'.
// Called by the embassy:expire_checks Asynq nightly sweep (§91, ADR-099).
func (s *Service) ExpireAlerts(ctx context.Context) (int, error) {
	n, err := s.store.ExpireAlerts(ctx)
	if err != nil {
		return 0, fmt.Errorf("appointmentalert: expire alerts: %w", err)
	}
	return int(n), nil
}

// PollEmbassy scrapes a monitored embassy's booking page for open slots.
// Stub: the HTTP scraper that respects robots.txt and writes availability
// state to Valkey (jc:embassy_avail:{slug}) is not yet implemented. Returns
// nil so the Asynq task succeeds gracefully without retrying (§91, ADR-099).
func (s *Service) PollEmbassy(ctx context.Context, embassyID, tenantID pgtype.UUID) error {
	// TODO(§91): implement robots.txt-aware scraper → Valkey availability
	// state → enqueue embassy:dispatch_alerts when slots are detected.
	return nil
}

// DispatchAlerts notifies all users with an active alert matching the
// embassy whose availability was detected. Stub: notification fan-out is
// not yet implemented. Returns nil so the Asynq task succeeds gracefully
// (§91, ADR-099).
func (s *Service) DispatchAlerts(ctx context.Context, embassySlug string, tenantID pgtype.UUID) error {
	// TODO(§91): query active alerts matching embassy_slug + tenant_id,
	// dispatch notifications via notification:dispatch_batch, set
	// fired_at = now(), and apply the 24-hour pause before re-firing.
	return nil
}

// ListUserAlerts returns a user's appointment alerts.
func (s *Service) ListUserAlerts(ctx context.Context, tenantID, userID pgtype.UUID, status string, page, limit int32) ([]AlertDTO, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, total, err := s.store.ListUserAlerts(ctx, ListUserAlertsFilter{
		TenantID: tenantID,
		UserID:   userID,
		Status:   status,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("appointmentalert: list user alerts: %w", err)
	}

	dtos := make([]AlertDTO, 0, len(rows))
	for _, r := range rows {
		dtos = append(dtos, alertToDTO(r))
	}
	return dtos, total, nil
}

// CreateAlert creates a new appointment alert for the given user.
func (s *Service) CreateAlert(ctx context.Context, tenantID, userID pgtype.UUID, input CreateAlertInput) (AlertDTO, error) {
	if input.EmbassySlug == "" {
		return AlertDTO{}, fmt.Errorf("embassy_slug is required")
	}
	if input.DestinationSpaceID == "" {
		return AlertDTO{}, fmt.Errorf("destination_space_id is required")
	}
	if input.DateRangeStart == "" {
		return AlertDTO{}, fmt.Errorf("date_range_start is required")
	}
	if input.DateRangeEnd == "" {
		return AlertDTO{}, fmt.Errorf("date_range_end is required")
	}

	var destSpaceID pgtype.UUID
	if err := destSpaceID.Scan(input.DestinationSpaceID); err != nil {
		return AlertDTO{}, fmt.Errorf("invalid destination_space_id: %w", err)
	}

	var dateStart pgtype.Date
	if err := dateStart.Scan(input.DateRangeStart); err != nil {
		return AlertDTO{}, fmt.Errorf("invalid date_range_start: %w", err)
	}
	var dateEnd pgtype.Date
	if err := dateEnd.Scan(input.DateRangeEnd); err != nil {
		return AlertDTO{}, fmt.Errorf("invalid date_range_end: %w", err)
	}

	var visaCat *string
	if input.VisaCategory != "" {
		v := input.VisaCategory
		visaCat = &v
	}

	var expiresAt pgtype.Date
	if input.ExpiresAt != "" {
		if err := expiresAt.Scan(input.ExpiresAt); err != nil {
			return AlertDTO{}, fmt.Errorf("invalid expires_at: %w", err)
		}
	}

	row, err := s.store.CreateAlert(ctx, AlertRow{
		TenantID:            tenantID,
		UserID:              userID,
		DestinationSpaceID:  destSpaceID,
		EmbassySlug:         input.EmbassySlug,
		VisaCategory:        visaCat,
		DateRangeStart:      dateStart,
		DateRangeEnd:        dateEnd,
		Status:              StatusActive,
		ExpiresAt:           expiresAt,
		NotificationChannels: input.NotificationChannels,
	})
	if err != nil {
		return AlertDTO{}, fmt.Errorf("appointmentalert: create alert: %w", err)
	}
	return alertToDTO(row), nil
}

// UpdateAlert updates a user's own appointment alert. The caller must
// verify ownership (user_id must match) before calling this method.
func (s *Service) UpdateAlert(ctx context.Context, id, tenantID, userID pgtype.UUID, input UpdateAlertInput) (AlertDTO, error) {
	existing, err := s.store.GetAlertByID(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AlertDTO{}, pgx.ErrNoRows
		}
		return AlertDTO{}, fmt.Errorf("appointmentalert: update alert: get: %w", err)
	}
	if existing.UserID != userID {
		return AlertDTO{}, ErrNotOwner
	}

	if input.Status != nil {
		if !isValidStatus(*input.Status) {
			return AlertDTO{}, fmt.Errorf("invalid status: %s", *input.Status)
		}
	}

	var visaCat *string
	if input.VisaCategory != nil {
		v := strings.TrimSpace(*input.VisaCategory)
		visaCat = &v
	}

	var dateStart *pgtype.Date
	if input.DateRangeStart != nil {
		var d pgtype.Date
		if err := d.Scan(*input.DateRangeStart); err != nil {
			return AlertDTO{}, fmt.Errorf("invalid date_range_start: %w", err)
		}
		dateStart = &d
	}

	var dateEnd *pgtype.Date
	if input.DateRangeEnd != nil {
		var d pgtype.Date
		if err := d.Scan(*input.DateRangeEnd); err != nil {
			return AlertDTO{}, fmt.Errorf("invalid date_range_end: %w", err)
		}
		dateEnd = &d
	}

	var expiresAt *pgtype.Date
	if input.ExpiresAt != nil {
		var d pgtype.Date
		if *input.ExpiresAt != "" {
			if err := d.Scan(*input.ExpiresAt); err != nil {
				return AlertDTO{}, fmt.Errorf("invalid expires_at: %w", err)
			}
		}
		expiresAt = &d
	}

	var channels []string
	if input.NotificationChannels != nil {
		channels = input.NotificationChannels
	}

	row, err := s.store.UpdateAlert(ctx, id, tenantID, input.Status, visaCat, dateStart, dateEnd, expiresAt, channels)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AlertDTO{}, pgx.ErrNoRows
		}
		return AlertDTO{}, fmt.Errorf("appointmentalert: update alert: %w", err)
	}
	return alertToDTO(row), nil
}

// DeleteAlert deletes a user's own appointment alert. The caller must
// verify ownership (user_id must match) before calling this method.
func (s *Service) DeleteAlert(ctx context.Context, id, tenantID, userID pgtype.UUID) error {
	existing, err := s.store.GetAlertByID(ctx, id, tenantID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("appointmentalert: delete alert: get: %w", err)
	}
	if existing.UserID != userID {
		return ErrNotOwner
	}
	if err := s.store.DeleteAlert(ctx, id, tenantID); err != nil {
		if err == pgx.ErrNoRows {
			return pgx.ErrNoRows
		}
		return fmt.Errorf("appointmentalert: delete alert: %w", err)
	}
	return nil
}

// isValidStatus reports whether s is one of the allowed alert statuses.
func isValidStatus(s string) bool {
	switch s {
	case StatusActive, StatusPaused, StatusFired, StatusExpired:
		return true
	}
	return false
}

// embassyToDTO converts an EmbassyRow to an EmbassyDTO.
func embassyToDTO(r EmbassyRow) EmbassyDTO {
	dto := EmbassyDTO{
		Slug:                r.Slug,
		Name:                r.Name,
		BookingURL:          r.BookingURL,
		PollIntervalSeconds: r.PollIntervalSeconds,
		IsActive:            r.IsActive,
		LastPolledAt:        formatTs(r.LastPolledAt),
		CreatedAt:           formatTs(r.CreatedAt),
		UpdatedAt:           formatTs(r.UpdatedAt),
	}
	if r.ID.Valid {
		dto.ID = r.ID.String()
	}
	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}
	if r.DestSpaceName != nil {
		dto.DestinationSpaceName = *r.DestSpaceName
	}
	return dto
}

// alertToDTO converts an AlertRow to an AlertDTO.
func alertToDTO(r AlertRow) AlertDTO {
	dto := AlertDTO{
		EmbassySlug:          r.EmbassySlug,
		Status:               r.Status,
		LastCheckedAt:        formatTs(r.LastCheckedAt),
		FiredAt:              formatTs(r.FiredAt),
		ExpiresAt:            formatDate(r.ExpiresAt),
		NotificationChannels: r.NotificationChannels,
		CreatedAt:            formatTs(r.CreatedAt),
		UpdatedAt:            formatTs(r.UpdatedAt),
	}
	if r.ID.Valid {
		dto.ID = r.ID.String()
	}
	if r.DestinationSpaceID.Valid {
		dto.DestinationSpaceID = r.DestinationSpaceID.String()
	}
	if r.DestSpaceName != nil {
		dto.DestinationSpaceName = *r.DestSpaceName
	}
	if r.VisaCategory != nil {
		dto.VisaCategory = *r.VisaCategory
	}
	dto.DateRangeStart = formatDate(r.DateRangeStart)
	dto.DateRangeEnd = formatDate(r.DateRangeEnd)
	if dto.NotificationChannels == nil {
		dto.NotificationChannels = []string{}
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

// formatDate renders a pgtype.Date as YYYY-MM-DD, or "" if NULL.
func formatDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}
