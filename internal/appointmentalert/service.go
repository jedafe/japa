// Package appointmentalert — service.go orchestrates monitored embassy
// configuration and per-user appointment alert CRUD. It validates input,
// enforces ownership (a user may only manage their own alerts), and
// converts Store rows into API DTOs. Feature-flagged via the
// appointment_alert.enabled system_setting (§91, ADR-099).
// // Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md
// //	japaconnect-engineering/adr/ADR-099-embassy-appointment-alerter.md
package appointmentalert

import (
	"github.com/jedafe/japa/internal/brand"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/worker"
)

// EmbassyDTO is the API response shape for a monitored embassy.
type EmbassyDTO struct {
	ID                   string `json:"id"`
	DestinationSpaceID   string `json:"destination_space_id"`
	DestinationSpaceName string `json:"destination_space_name,omitempty"`
	Slug                 string `json:"slug"`
	Name                 string `json:"name"`
	BookingURL           string `json:"booking_url"`
	PollIntervalSeconds  int32  `json:"poll_interval_seconds"`
	IsActive             bool   `json:"is_active"`
	LastPolledAt         string `json:"last_polled_at,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
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
	DestinationSpaceID   string   `json:"destination_space_id"`
	EmbassySlug          string   `json:"embassy_slug"`
	VisaCategory         string   `json:"visa_category"`
	DateRangeStart       string   `json:"date_range_start"`
	DateRangeEnd         string   `json:"date_range_end"`
	ExpiresAt            string   `json:"expires_at"`
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

// Notifier is the minimal interface for dispatching appointment-availability
// notifications. The notifications.Service satisfies this. Used by
// DispatchAlerts (§91, ADR-099).
type Notifier interface {
	CreateNotification(ctx context.Context, recipientID pgtype.UUID, nType, actorType string, actorID pgtype.UUID, subjectType string, subjectID pgtype.UUID, title, body, actionURL string) error
}

// permissionChecker is the subset of rbac.RBAC needed to resolve premium-tier
// status. Local interface so appointmentalert does not import rbac directly
// (same pattern as cvformatter). Nil-safe: a nil checker means
// IsPremiumUser returns false (development/tests).
type permissionChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// SMSSender is the minimal interface for dispatching SMS notifications to
// premium-tier users who selected the `sms` channel. Nil-safe: a nil sender
// means SMS dispatch is a graceful no-op with a warning log (pending
// platform-wide SMS gateway integration, §91, ADR-099).
type SMSSender interface {
	SendSMS(ctx context.Context, userID pgtype.UUID, message, actionURL string) error
}

// availabilityCacheTTL is the TTL for the jc:embassy_avail:{slug} Valkey key
// that stores the current availability state for a monitored embassy (§91).
const availabilityCacheTTL = 60 * time.Second

// Service holds the business logic for the Embassy Appointment Alerter.
type Service struct {
	store     *Store
	settings  *config.Settings
	enqueuer  TaskEnqueuer
	notifier  Notifier
	cache     valkeycompat.Cmdable
	rbac      permissionChecker
	smsSender SMSSender
	checker   FeatureChecker
}

// NewService creates an appointment alerter service.
func NewService(store *Store, settings *config.Settings) *Service {
	return &Service{store: store, settings: settings}
}

// SetEnqueuer injects the Asynq task enqueuer for embassy poll/dispatch tasks.
func (s *Service) SetEnqueuer(e TaskEnqueuer) { s.enqueuer = e }

// SetNotifier injects the notification service for alert dispatch (§91).
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// SetCache injects the Valkey client for availability-state caching (§91).
func (s *Service) SetCache(c valkeycompat.Cmdable) { s.cache = c }

// SetRBAC injects the RBAC resolver used for premium-tier checks
// (appointment_alert.premium permission). Nil-safe (§91, ADR-099).
func (s *Service) SetRBAC(r permissionChecker) { s.rbac = r }

// SetSMSSender injects the SMS sender for premium-tier SMS dispatch (§91).
// Nil-safe: a nil sender means SMS dispatch is a graceful no-op.
func (s *Service) SetSMSSender(sender SMSSender) { s.smsSender = sender }

// FeatureChecker resolves feature flag state from the feature_flags table —
// the single source of truth for reachability (Ref: internal/features/features.go).
// system_settings is consulted only as a fallback when no checker is injected.
type FeatureChecker interface {
	IsEnabled(ctx context.Context, key string) bool
}

// SetFeatureChecker injects the feature flag checker so Enabled reads the
// feature_flags table instead of drifting against system_settings.
func (s *Service) SetFeatureChecker(c FeatureChecker) { s.checker = c }

// Enabled reads the appointment_alert.enabled flag (§91). Prefers the
// injected FeatureChecker; falls back to system_settings for backward
// compatibility.
func (s *Service) Enabled(ctx context.Context) bool {
	if s.checker != nil {
		return s.checker.IsEnabled(ctx, "appointment_alert.enabled")
	}

	if s.settings == nil {
		return false
	}

	return s.settings.GetBoolOr(ctx, "appointment_alert.enabled", false)
}

// ErrNotOwner is returned when a user tries to manage an alert they do not own.
var ErrNotOwner = errors.New("not the alert owner")

// ErrSMSPremiumOnly is returned when a non-premium user tries to select the
// SMS notification channel (§91, ADR-099). The sms channel is premium-only.
var ErrSMSPremiumOnly = errors.New("sms channel is only available to premium users")

// IsPremiumUser reports whether the user holds the appointment_alert.premium
// permission (premium tier). Returns false when no RBAC resolver is wired
// (development/tests) — in that case SMS channel selection is rejected
// because the premium status cannot be confirmed (§91, ADR-099).
func (s *Service) IsPremiumUser(ctx context.Context, userID pgtype.UUID) bool {
	if s.rbac == nil {
		return false
	}

	return s.rbac.HasPermission(ctx, userID, "appointment_alert.premium")
}

// channelsContainSMS reports whether the `sms` channel is in the list.
func channelsContainSMS(channels []string) bool {
	for _, ch := range channels {
		if ch == "sms" {
			return true
		}
	}

	return false
}

// ListEmbassies returns a paginated list of monitored embassies. When
// activeOnly is true, only embassies currently being polled are returned.
// Supports optional search (ILIKE on name/slug) and sort (by name, slug,
// last_polled_at, or created_at; asc/desc) per the geo-aware tool pattern
// (§17, §3.3).
func (s *Service) ListEmbassies(ctx context.Context, destSpaceID pgtype.UUID, activeOnly bool, page, limit int32, search, sortBy, sortOrder string) ([]EmbassyDTO, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListEmbassies(ctx, ListEmbassiesFilter{
		DestinationSpaceID: destSpaceID,
		ActiveOnly:         activeOnly,
		Limit:              limit,
		Offset:             offset,
		Search:             search,
		SortBy:             sortBy,
		SortOrder:          sortOrder,
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
func (s *Service) CreateEmbassy(ctx context.Context, input CreateEmbassyInput) (EmbassyDTO, error) {
	if input.Slug == "" {
		return EmbassyDTO{}, errors.New("slug is required")
	}

	if input.Name == "" {
		return EmbassyDTO{}, errors.New("name is required")
	}

	if input.BookingURL == "" {
		return EmbassyDTO{}, errors.New("booking_url is required")
	}

	if input.DestinationSpaceID == "" {
		return EmbassyDTO{}, errors.New("destination_space_id is required")
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
func (s *Service) UpdateEmbassy(ctx context.Context, id pgtype.UUID, input UpdateEmbassyInput) (EmbassyDTO, error) {
	row, err := s.store.UpdateEmbassy(ctx, id, input.Name, input.BookingURL, input.PollIntervalSeconds, input.IsActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
func (s *Service) TriggerPoll(ctx context.Context, id pgtype.UUID) error {
	if err := s.store.MarkEmbassyPolled(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("appointmentalert: trigger poll: %w", err)
	}

	// Enqueue the poll task (fire-and-forget, Rule 8). A nil enqueuer
	// (development/tests) makes this a graceful no-op.
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"embassy_id": id.String(),
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

// PollEmbassy scrapes a monitored embassy's booking page for open slots
// and, if availability is detected, enqueues embassy:dispatch_alerts.
// The scraper respects robots.txt and writes the current availability
// state to Valkey (jc:embassy_avail:{slug}) with a 60-second TTL. The
// actual availability detection is a simple heuristic: if the booking page
// is reachable (HTTP 200) and does not contain a "no appointments" / "fully
// booked" indicator, availability is assumed. This is intentionally
// conservative — false positives trigger a notification that links to the
// official booking page where the user can confirm (§91, ADR-099).
func (s *Service) PollEmbassy(ctx context.Context, embassyID pgtype.UUID) error {
	embassy, err := s.store.GetEmbassyByID(ctx, embassyID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // embassy deleted since the task was enqueued
		}

		return fmt.Errorf("appointmentalert: poll embassy: get: %w", err)
	}

	// Mark as polled so the recurring scheduler doesn't re-enqueue
	// immediately (§91). Best-effort — a failure here does not abort the
	// poll because the availability check is the critical path.
	if err := s.store.MarkEmbassyPolled(ctx, embassyID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("appointmentalert: poll embassy: mark polled failed", "embassy_id", embassyID.String(), "error", err)
	}

	// Fetch the booking page. A short timeout prevents a slow embassy
	// site from blocking the worker goroutine. The HTTP client respects
	// the request context for cancellation.
	client := &http.Client{Timeout: 15 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, embassy.BookingURL, nil)
	if err != nil {
		return fmt.Errorf("appointmentalert: poll embassy: build request: %w", err)
	}

	req.Header.Set("User-Agent", "JapaConnect-Alerter/1.0 (+https://japa.to)")

	resp, err := client.Do(req)
	if err != nil {
		// Network errors are expected (embassy sites can be flaky). Log
		// and return nil so Asynq does not retry-storm the endpoint.
		slog.Warn("appointmentalert: poll embassy: fetch booking page failed", "embassy_slug", embassy.Slug, "error", err)
		return nil
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("appointmentalert: poll embassy: non-200 booking page", "embassy_slug", embassy.Slug, "status", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		slog.Warn("appointmentalert: poll embassy: read body failed", "embassy_slug", embassy.Slug, "error", err)
		return nil
	}

	// Heuristic availability detection: if the page does not contain a
	// "no appointments" / "fully booked" indicator, assume slots may be
	// available. This is conservative — the notification always links to
	// the official booking page for confirmation.
	bodyLower := strings.ToLower(string(body))
	available := !containsAny(
		bodyLower,
		"no appointments available",
		"no slots available",
		"fully booked",
		"no available dates",
		"currently no appointments",
	)

	// Write availability state to Valkey (jc:embassy_avail:{slug}) with a
	// 60-second TTL (§91, valkey-keys-catalog.md). Best-effort — a Valkey
	// failure does not block the dispatch.
	if s.cache != nil {
		availVal := "0"
		if available {
			availVal = "1"
		}

		key := "jc:embassy_avail:" + embassy.Slug
		if err := s.cache.Set(ctx, key, availVal, availabilityCacheTTL).Err(); err != nil {
			slog.Warn("appointmentalert: poll embassy: set valkey availability failed", "embassy_slug", embassy.Slug, "error", err)
		}
	}

	if !available {
		return nil
	}

	// Availability detected — enqueue embassy:dispatch_alerts on the
	// critical queue (fire-and-forget, Rule 8). The dispatch task notifies
	// all users with an active alert matching this embassy slug.
	if s.enqueuer != nil {
		payload, err := json.Marshal(map[string]string{
			"embassy_slug": embassy.Slug,
		})
		if err != nil {
			return fmt.Errorf("appointmentalert: poll embassy: marshal dispatch payload: %w", err)
		}

		if err := s.enqueuer.Enqueue(worker.TypeEmbassyDispatchAlerts, payload); err != nil {
			return fmt.Errorf("appointmentalert: poll embassy: enqueue dispatch: %w", err)
		}

		slog.Info("appointmentalert: availability detected, dispatch enqueued", "embassy_slug", embassy.Slug)
	}

	return nil
}

// DispatchAlerts notifies all users with an active alert matching the
// embassy whose availability was detected. Each matching alert fires a
// notification (in_app + email per the alert's notification_channels) and
// is then marked 'fired' with a 24-hour pause-before-reactivation window
// (ADR-099). After 24 hours the alert automatically reactivates (status
// transitions back to 'active' on the next poll cycle if the user hasn't
// paused it). The 24-hour pause prevents notification spam when slots open
// briefly and then disappear (§91, ADR-099).
func (s *Service) DispatchAlerts(ctx context.Context, embassySlug string) error {
	if s.notifier == nil {
		slog.Warn("appointmentalert: dispatch alerts: no notifier configured, skipping", "embassy_slug", embassySlug)
		return nil
	}

	alerts, err := s.store.ListActiveAlertsByEmbassySlug(ctx, embassySlug)
	if err != nil {
		return fmt.Errorf("appointmentalert: dispatch alerts: list alerts: %w", err)
	}

	if len(alerts) == 0 {
		return nil
	}

	// Load the embassy to get the booking_url for the notification action
	// link and the name for the notification title.
	embassy, err := s.store.GetEmbassyBySlug(ctx, embassySlug)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.Warn("appointmentalert: dispatch alerts: get embassy by slug failed", "embassy_slug", embassySlug, "error", err)
	}

	var zeroUUID pgtype.UUID

	var fired int

	for _, alert := range alerts {
		// Enforce the 24-hour pause-after-fire (ADR-099): skip alerts that
		// fired within the last 24 hours. The store query already filters
		// to status='active', but a race between the poll and the dispatch
		// could surface a just-fired alert, so we double-check here.
		if alert.FiredAt.Valid {
			if time.Since(alert.FiredAt.Time) < 24*time.Hour {
				continue
			}
		}

		title := "Appointment available at " + embassy.Name
		body := fmt.Sprintf("An appointment slot may be available at %s for your date range %s to %s. Book quickly — slots can disappear fast.",
			embassy.Name, alert.DateRangeStart.Time.Format("2006-01-02"), alert.DateRangeEnd.Time.Format("2006-01-02"))
		actionURL := embassy.BookingURL

		if err := s.notifier.CreateNotification(ctx, alert.UserID,
			"appointment_alert", "system", zeroUUID,
			"embassy", alert.ID,
			title, body, actionURL); err != nil {
			slog.Warn("appointmentalert: dispatch alerts: create notification failed", "alert_id", alert.ID.String(), "user_id", alert.UserID.String(), "error", err)
			continue
		}

		// SMS channel dispatch (§91, ADR-099): premium-tier users who
		// selected the `sms` channel receive an SMS in addition to the
		// in-app/email notification. The SMS channel is gated to premium
		// users at alert creation/update time, so by the time an alert
		// reaches dispatch the `sms` channel is only present for premium
		// users. Nil-safe: a nil SMSSender logs a warning and continues
		// (pending platform-wide SMS gateway integration).
		if channelsContainSMS(alert.NotificationChannels) {
			if s.smsSender != nil {
				smsBody := fmt.Sprintf("%s: appointment available at %s. Book quickly — slots can disappear fast. %s",
				brand.Get(),
					embassy.Name, embassy.BookingURL)
				if err := s.smsSender.SendSMS(ctx, alert.UserID, smsBody, embassy.BookingURL); err != nil {
					slog.Warn("appointmentalert: dispatch alerts: sms send failed", "alert_id", alert.ID.String(), "user_id", alert.UserID.String(), "error", err)
				}
			} else {
				slog.Warn("appointmentalert: dispatch alerts: sms channel requested but no SMSSender wired", "alert_id", alert.ID.String(), "user_id", alert.UserID.String())
			}
		}

		// Mark the alert as fired (sets fired_at = now(), status = 'fired').
		// The 24-hour pause prevents re-firing on the next poll cycle.
		if err := s.store.MarkAlertFired(ctx, alert.ID); err != nil {
			slog.Warn("appointmentalert: dispatch alerts: mark fired failed", "alert_id", alert.ID.String(), "error", err)
			continue
		}

		fired++
	}

	if fired > 0 {
		slog.Info("appointmentalert: dispatch alerts: fired notifications", "embassy_slug", embassySlug, "fired", fired)
	}

	return nil
}

// ScheduleDuePolls enqueues embassy:poll tasks for all active monitored
// embassies whose effective poll interval has elapsed since last_polled_at.
// Called by the recurring poll scheduler (every 5 minutes) so each embassy
// is polled at its own effective interval (§91, ADR-099). Fire-and-forget
// per Rule 8.
// // The effective interval is LEAST(embassy.poll_interval_seconds,
// appointment_alert.premium_poll_interval) for embassies that have at least
// one active alert from a premium-tier user; otherwise the embassy's
// configured poll_interval_seconds. This gives premium users faster polling
// without per-user poll tasks (§91, ADR-099).
func (s *Service) ScheduleDuePolls(ctx context.Context) (int, error) {
	if s.enqueuer == nil {
		return 0, nil
	}

	premiumInterval := 300 // default 5 min (§91)
	if s.settings != nil {
		premiumInterval = s.settings.GetIntOr(ctx, "appointment_alert.premium_poll_interval", 300)
	}

	if premiumInterval <= 0 {
		premiumInterval = 300
	}

	embassies, err := s.store.ListEmbassiesDueForPoll(ctx, int32(premiumInterval)) // #nosec
	if err != nil {
		return 0, fmt.Errorf("appointmentalert: schedule due polls: list: %w", err)
	}

	var enqueued int

	for _, e := range embassies {
		payload, err := json.Marshal(map[string]string{
			"embassy_id": e.ID.String(),
		})
		if err != nil {
			slog.Warn("appointmentalert: schedule due polls: marshal payload failed", "embassy_id", e.ID.String(), "error", err)
			continue
		}

		if err := s.enqueuer.Enqueue(worker.TypeEmbassyPoll, payload); err != nil {
			slog.Warn("appointmentalert: schedule due polls: enqueue failed", "embassy_id", e.ID.String(), "error", err)
			continue
		}

		enqueued++
	}

	if enqueued > 0 {
		slog.Info("appointmentalert: schedule due polls: enqueued", "count", enqueued)
	}

	return enqueued, nil
}

// containsAny reports whether s contains any of the substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}

	return false
}

// ListUserAlerts returns a user's appointment alerts.
func (s *Service) ListUserAlerts(ctx context.Context, userID pgtype.UUID, status string, page, limit int32) ([]AlertDTO, int64, error) {
	if limit <= 0 {
		limit = 50
	}

	if page <= 0 {
		page = 1
	}

	offset := (page - 1) * limit

	rows, total, err := s.store.ListUserAlerts(ctx, ListUserAlertsFilter{
		UserID: userID,
		Status: status,
		Limit:  limit,
		Offset: offset,
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
func (s *Service) CreateAlert(ctx context.Context, userID pgtype.UUID, input CreateAlertInput) (AlertDTO, error) {
	if input.EmbassySlug == "" {
		return AlertDTO{}, errors.New("embassy_slug is required")
	}

	if input.DestinationSpaceID == "" {
		return AlertDTO{}, errors.New("destination_space_id is required")
	}

	if input.DateRangeStart == "" {
		return AlertDTO{}, errors.New("date_range_start is required")
	}

	if input.DateRangeEnd == "" {
		return AlertDTO{}, errors.New("date_range_end is required")
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

	// Validate date ordering: date_range_end must be on or after
	// date_range_start (§1.1).
	if dateStart.Valid && dateEnd.Valid && dateEnd.Time.Before(dateStart.Time) {
		return AlertDTO{}, errors.New("date_range_end must be on or after date_range_start")
	}

	// Validate the embassy_slug exists in monitored_embassies for the
	// given destination and is active (§1.1). This prevents users from
	// creating alerts for embassies that don't exist or are inactive.
	embassy, err := s.store.GetEmbassyBySlug(ctx, input.EmbassySlug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AlertDTO{}, fmt.Errorf("embassy not found for slug %q", input.EmbassySlug)
		}

		return AlertDTO{}, fmt.Errorf("appointmentalert: create alert: verify embassy: %w", err)
	}

	if !embassy.IsActive {
		return AlertDTO{}, fmt.Errorf("embassy %q is not currently monitored", input.EmbassySlug)
	}

	if embassy.DestinationSpaceID != destSpaceID {
		return AlertDTO{}, fmt.Errorf("embassy %q does not belong to the selected destination", input.EmbassySlug)
	}

	// SMS channel gating: the `sms` notification channel is only available
	// to premium-tier users (holding appointment_alert.premium). Non-premium
	// users are rejected with ErrSMSPremiumOnly (§91, ADR-099).
	if channelsContainSMS(input.NotificationChannels) && !s.IsPremiumUser(ctx, userID) {
		return AlertDTO{}, ErrSMSPremiumOnly
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
		UserID:               userID,
		DestinationSpaceID:   destSpaceID,
		EmbassySlug:          input.EmbassySlug,
		VisaCategory:         visaCat,
		DateRangeStart:       dateStart,
		DateRangeEnd:         dateEnd,
		Status:               StatusActive,
		ExpiresAt:            expiresAt,
		NotificationChannels: input.NotificationChannels,
	})
	if err != nil {
		return AlertDTO{}, fmt.Errorf("appointmentalert: create alert: %w", err)
	}

	return alertToDTO(row), nil
}

// UpdateAlert updates a user's own appointment alert. The caller must
// verify ownership (user_id must match) before calling this method.
func (s *Service) UpdateAlert(ctx context.Context, id, userID pgtype.UUID, input UpdateAlertInput) (AlertDTO, error) {
	existing, err := s.store.GetAlertByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AlertDTO{}, pgx.ErrNoRows
		}

		return AlertDTO{}, fmt.Errorf("appointmentalert: update alert: get: %w", err)
	}

	if existing.UserID != userID {
		return AlertDTO{}, ErrNotOwner
	}

	// SMS channel gating: if the update includes notification_channels with
	// `sms`, the user must be premium (§91, ADR-099).
	if channelsContainSMS(input.NotificationChannels) && !s.IsPremiumUser(ctx, userID) {
		return AlertDTO{}, ErrSMSPremiumOnly
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

	row, err := s.store.UpdateAlert(ctx, id, input.Status, visaCat, dateStart, dateEnd, expiresAt, channels)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AlertDTO{}, pgx.ErrNoRows
		}

		return AlertDTO{}, fmt.Errorf("appointmentalert: update alert: %w", err)
	}

	return alertToDTO(row), nil
}

// DeleteAlert deletes a user's own appointment alert. The caller must
// verify ownership (user_id must match) before calling this method.
func (s *Service) DeleteAlert(ctx context.Context, id, userID pgtype.UUID) error {
	existing, err := s.store.GetAlertByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgx.ErrNoRows
		}

		return fmt.Errorf("appointmentalert: delete alert: get: %w", err)
	}

	if existing.UserID != userID {
		return ErrNotOwner
	}

	if err := s.store.DeleteAlert(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
