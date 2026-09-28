// Package appointmentalert implements the Embassy Appointment Alerter
// (§91, ADR-099).
// // The package manages two tables created by migration 000327:
// monitored_embassies (admin-configured embassy booking pages that the
// Asynq worker polls for open appointment slots) and appointment_alerts
// (per-user subscriptions that fire a notification when a matching slot
// appears). This package provides the CRUD, listing, and admin config
// endpoints; the actual scraping/polling is performed by an Asynq worker
// outside this package.
// // Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md
// //	japaconnect-engineering/adr/ADR-099-embassy-appointment-alerter.md
package appointmentalert

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Alert statuses (§91).
const (
	StatusActive  = "active"
	StatusPaused  = "paused"
	StatusFired   = "fired"
	StatusExpired = "expired"
)

// Default notification channels when none are specified.
var defaultNotificationChannels = []string{"in_app", "email"}

// Store is the data-access layer for monitored_embassies and
// appointment_alerts. It uses direct pgx queries because these are new
// tables without sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates an appointment alerter store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// EmbassyRow is a row from monitored_embassies, optionally joined to
// spaces for the destination name.
type EmbassyRow struct {
	ID pgtype.UUID

	DestinationSpaceID  pgtype.UUID
	DestSpaceName       *string
	Slug                string
	Name                string
	BookingURL          string
	PollIntervalSeconds int32
	IsActive            bool
	LastPolledAt        pgtype.Timestamptz
	CreatedAt           pgtype.Timestamptz
	UpdatedAt           pgtype.Timestamptz
}

// AlertRow is a row from appointment_alerts.
type AlertRow struct {
	ID pgtype.UUID

	UserID               pgtype.UUID
	DestinationSpaceID   pgtype.UUID
	DestSpaceName        *string
	EmbassySlug          string
	VisaCategory         *string
	DateRangeStart       pgtype.Date
	DateRangeEnd         pgtype.Date
	Status               string
	LastCheckedAt        pgtype.Timestamptz
	FiredAt              pgtype.Timestamptz
	ExpiresAt            pgtype.Date
	NotificationChannels []string
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
	DeletedAt            pgtype.Timestamptz
}

// ListEmbassiesFilter holds the filter parameters for listing monitored
// embassies.
type ListEmbassiesFilter struct {
	DestinationSpaceID pgtype.UUID // optional
	ActiveOnly         bool
	Limit              int32
	Offset             int32
	Search             string // optional ILIKE search on name/slug
	SortBy             string // optional: "name", "slug", "last_polled_at", "created_at"
	SortOrder          string // optional: "asc" or "desc" (default "asc")
}

// ListEmbassies returns a paginated list of monitored embassies for a
// platform, optionally filtered by destination space. When ActiveOnly is
// true only rows with is_active = true are returned.
func (s *Store) ListEmbassies(ctx context.Context, f ListEmbassiesFilter) ([]EmbassyRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("e.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.ActiveOnly {
		conditions = append(conditions, "e.is_active = true")
	}

	// Optional ILIKE search on name or slug (§17, geo-aware tool pattern).
	if f.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(e.name ILIKE $%d OR e.slug ILIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+f.Search+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
	}

	// Sort column whitelist (prevent SQL injection — never interpolate
	// user input directly into ORDER BY).
	sortCol := "e.name"

	switch f.SortBy {
	case "name":
		sortCol = "e.name"
	case "slug":
		sortCol = "e.slug"
	case "last_polled_at":
		sortCol = "e.last_polled_at"
	case "created_at":
		sortCol = "e.created_at"
	}

	sortDir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		sortDir = "DESC"
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	if limit > 200 {
		limit = 200
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQuery := fmt.Sprintf(`
SELECT e.id, e.destination_space_id, sp.name AS dest_space_name,
       e.slug, e.name, e.booking_url, e.poll_interval_seconds, e.is_active,
       e.last_polled_at, e.created_at, e.updated_at
FROM monitored_embassies e
LEFT JOIN spaces sp ON sp.id = e.destination_space_id
WHERE %s
ORDER BY %s %s
LIMIT $%d OFFSET $%d
`, whereClause, sortCol, sortDir, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("appointmentalert: list embassies: %w", err)
	}
	defer rows.Close()

	var out []EmbassyRow

	for rows.Next() {
		var r EmbassyRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID, &r.DestSpaceName,
			&r.Slug, &r.Name, &r.BookingURL, &r.PollIntervalSeconds, &r.IsActive,
			&r.LastPolledAt, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("appointmentalert: scan embassy row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("appointmentalert: iterate embassy rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM monitored_embassies e
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("appointmentalert: count embassies: %w", err)
	}

	return out, total, nil
}

// GetEmbassyBySlug returns a single monitored embassy by its slug within
// the platform.
func (s *Store) GetEmbassyBySlug(ctx context.Context, slug string) (EmbassyRow, error) {
	const q = `
SELECT e.id, e.destination_space_id, sp.name AS dest_space_name,
       e.slug, e.name, e.booking_url, e.poll_interval_seconds, e.is_active,
       e.last_polled_at, e.created_at, e.updated_at
FROM monitored_embassies e
LEFT JOIN spaces sp ON sp.id = e.destination_space_id
WHERE e.slug = $1
LIMIT 1
`

	var r EmbassyRow

	err := s.pool.Reader.QueryRow(ctx, q, slug).Scan(
		&r.ID, &r.DestinationSpaceID, &r.DestSpaceName,
		&r.Slug, &r.Name, &r.BookingURL, &r.PollIntervalSeconds, &r.IsActive,
		&r.LastPolledAt, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EmbassyRow{}, pgx.ErrNoRows
		}

		return EmbassyRow{}, fmt.Errorf("appointmentalert: get embassy by slug: %w", err)
	}

	return r, nil
}

// CreateEmbassy inserts a new monitored_embassies row.
func (s *Store) CreateEmbassy(ctx context.Context, r EmbassyRow) (EmbassyRow, error) {
	const q = `
INSERT INTO monitored_embassies (
    destination_space_id, slug, name, booking_url,
    poll_interval_seconds, is_active
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, destination_space_id, slug, name, booking_url,
          poll_interval_seconds, is_active, last_polled_at, created_at, updated_at
`

	var out EmbassyRow

	err := s.pool.Writer.QueryRow(
		ctx, q, (pgtype.UUID{}),

		r.DestinationSpaceID, r.Slug, r.Name, r.BookingURL,
		r.PollIntervalSeconds, r.IsActive,
	).Scan(
		&out.ID, &out.DestinationSpaceID, &out.Slug, &out.Name,
		&out.BookingURL, &out.PollIntervalSeconds, &out.IsActive,
		&out.LastPolledAt, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		return EmbassyRow{}, fmt.Errorf("appointmentalert: create embassy: %w", err)
	}

	return out, nil
}

// UpdateEmbassy updates the mutable config fields on a monitored_embassies
// row. Only non-nil/non-zero fields in the input are applied.
func (s *Store) UpdateEmbassy(ctx context.Context, id pgtype.UUID, name, bookingURL *string, pollInterval *int32, isActive *bool) (EmbassyRow, error) {
	var (
		sets []string
		args []any
		idx  = 1
	)

	args = append(args, id)
	idx = 3

	if name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, *name)
		idx++
	}

	if bookingURL != nil {
		sets = append(sets, fmt.Sprintf("booking_url = $%d", idx))
		args = append(args, *bookingURL)
		idx++
	}

	if pollInterval != nil {
		sets = append(sets, fmt.Sprintf("poll_interval_seconds = $%d", idx))
		args = append(args, *pollInterval)
		idx++
	}

	if isActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", idx))
		args = append(args, *isActive)
		idx++
	}

	if len(sets) == 0 {
		// Nothing to update; just return the current row.
		return s.getEmbassyByID(ctx, id)
	}

	sets = append(sets, "updated_at = now()")

	q := fmt.Sprintf(`
UPDATE monitored_embassies
SET %s
WHERE id = $1
RETURNING id, destination_space_id, slug, name, booking_url,
          poll_interval_seconds, is_active, last_polled_at, created_at, updated_at
`, strings.Join(sets, ", "))

	var out EmbassyRow

	err := s.pool.Writer.QueryRow(ctx, q, args...).Scan(
		&out.ID, &out.DestinationSpaceID, &out.Slug, &out.Name,
		&out.BookingURL, &out.PollIntervalSeconds, &out.IsActive,
		&out.LastPolledAt, &out.CreatedAt, &out.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EmbassyRow{}, pgx.ErrNoRows
		}

		return EmbassyRow{}, fmt.Errorf("appointmentalert: update embassy: %w", err)
	}

	return out, nil
}

// getEmbassyByID fetches a single monitored embassy by its PK.
func (s *Store) getEmbassyByID(ctx context.Context, id pgtype.UUID) (EmbassyRow, error) {
	const q = `
SELECT e.id, e.destination_space_id, sp.name AS dest_space_name,
       e.slug, e.name, e.booking_url, e.poll_interval_seconds, e.is_active,
       e.last_polled_at, e.created_at, e.updated_at
FROM monitored_embassies e
LEFT JOIN spaces sp ON sp.id = e.destination_space_id
WHERE e.id = $1
LIMIT 1
`

	var r EmbassyRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.DestinationSpaceID, &r.DestSpaceName,
		&r.Slug, &r.Name, &r.BookingURL, &r.PollIntervalSeconds, &r.IsActive,
		&r.LastPolledAt, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EmbassyRow{}, pgx.ErrNoRows
		}

		return EmbassyRow{}, fmt.Errorf("appointmentalert: get embassy by id: %w", err)
	}

	return r, nil
}

// MarkEmbassyPolled sets last_polled_at = now() for the given embassy.
// The actual scraping is performed by the Asynq worker; this just records
// that a poll cycle has been triggered.
func (s *Store) MarkEmbassyPolled(ctx context.Context, id pgtype.UUID) error {
	const q = `
UPDATE monitored_embassies
SET last_polled_at = now(), updated_at = now()
WHERE id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("appointmentalert: mark embassy polled: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ListUserAlertsFilter holds the filter parameters for a user's alerts.
type ListUserAlertsFilter struct {
	UserID pgtype.UUID
	Status string // optional: active|paused|fired|expired
	Limit  int32
	Offset int32
}

// ListUserAlerts returns a paginated list of a user's appointment alerts.
func (s *Store) ListUserAlerts(ctx context.Context, f ListUserAlertsFilter) ([]AlertRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("a.user_id = $%d", argIdx))
	args = append(args, f.UserID)
	argIdx++

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("a.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	// Rule 11: never return soft-deleted rows.
	conditions = append(conditions, "a.deleted_at IS NULL")

	whereClause := strings.Join(conditions, " AND ")
	if whereClause == "" {
		whereClause = "TRUE"
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	if limit > 200 {
		limit = 200
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	listQuery := fmt.Sprintf(`
SELECT a.id, a.user_id, a.destination_space_id, sp.name AS dest_space_name,
       a.embassy_slug, a.visa_category, a.date_range_start, a.date_range_end,
       a.status, a.last_checked_at, a.fired_at, a.expires_at,
       a.notification_channels, a.created_at, a.updated_at, a.deleted_at
FROM appointment_alerts a
LEFT JOIN spaces sp ON sp.id = a.destination_space_id
WHERE %s
ORDER BY a.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("appointmentalert: list user alerts: %w", err)
	}
	defer rows.Close()

	var out []AlertRow

	for rows.Next() {
		var r AlertRow
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.DestinationSpaceID, &r.DestSpaceName,
			&r.EmbassySlug, &r.VisaCategory, &r.DateRangeStart, &r.DateRangeEnd,
			&r.Status, &r.LastCheckedAt, &r.FiredAt, &r.ExpiresAt,
			&r.NotificationChannels, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("appointmentalert: scan alert row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("appointmentalert: iterate alert rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM appointment_alerts a
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("appointmentalert: count user alerts: %w", err)
	}

	return out, total, nil
}

// GetAlertByID returns a single appointment alert by its PK.
func (s *Store) GetAlertByID(ctx context.Context, id pgtype.UUID) (AlertRow, error) {
	const q = `
SELECT a.id, a.user_id, a.destination_space_id, sp.name AS dest_space_name,
       a.embassy_slug, a.visa_category, a.date_range_start, a.date_range_end,
       a.status, a.last_checked_at, a.fired_at, a.expires_at,
       a.notification_channels, a.created_at, a.updated_at, a.deleted_at
FROM appointment_alerts a
LEFT JOIN spaces sp ON sp.id = a.destination_space_id
WHERE a.id = $1 AND a.deleted_at IS NULL
LIMIT 1
`

	var r AlertRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.UserID, &r.DestinationSpaceID, &r.DestSpaceName,
		&r.EmbassySlug, &r.VisaCategory, &r.DateRangeStart, &r.DateRangeEnd,
		&r.Status, &r.LastCheckedAt, &r.FiredAt, &r.ExpiresAt,
		&r.NotificationChannels, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AlertRow{}, pgx.ErrNoRows
		}

		return AlertRow{}, fmt.Errorf("appointmentalert: get alert by id: %w", err)
	}

	return r, nil
}

// CreateAlert inserts a new appointment_alerts row and returns the full row.
func (s *Store) CreateAlert(ctx context.Context, r AlertRow) (AlertRow, error) {
	channels := r.NotificationChannels
	if len(channels) == 0 {
		channels = defaultNotificationChannels
	}

	status := r.Status
	if status == "" {
		status = StatusActive
	}

	const q = `
INSERT INTO appointment_alerts (
    user_id, destination_space_id, embassy_slug, visa_category,
    date_range_start, date_range_end, status, expires_at, notification_channels
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, user_id, destination_space_id, embassy_slug, visa_category,
          date_range_start, date_range_end, status, last_checked_at, fired_at,
          expires_at, notification_channels, created_at, updated_at, deleted_at
`

	var out AlertRow

	err := s.pool.Writer.QueryRow(
		ctx, q, (pgtype.UUID{}),

		r.UserID, r.DestinationSpaceID, r.EmbassySlug, r.VisaCategory,
		r.DateRangeStart, r.DateRangeEnd, status, r.ExpiresAt, channels,
	).Scan(
		&out.ID, &out.UserID, &out.DestinationSpaceID,
		&out.EmbassySlug, &out.VisaCategory, &out.DateRangeStart, &out.DateRangeEnd,
		&out.Status, &out.LastCheckedAt, &out.FiredAt, &out.ExpiresAt,
		&out.NotificationChannels, &out.CreatedAt, &out.UpdatedAt, &out.DeletedAt,
	)
	if err != nil {
		return AlertRow{}, fmt.Errorf("appointmentalert: create alert: %w", err)
	}

	return out, nil
}

// UpdateAlert updates the mutable fields on an appointment_alerts row.
// Only non-nil fields in the input are applied. Returns the updated row.
func (s *Store) UpdateAlert(ctx context.Context, id pgtype.UUID, status, visaCategory *string, dateRangeStart, dateRangeEnd, expiresAt *pgtype.Date, channels []string) (AlertRow, error) {
	var (
		sets []string
		args []any
		idx  = 1
	)

	args = append(args, id)
	idx = 3

	if status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", idx))
		args = append(args, *status)
		idx++
	}

	if visaCategory != nil {
		sets = append(sets, fmt.Sprintf("visa_category = $%d", idx))
		args = append(args, *visaCategory)
		idx++
	}

	if dateRangeStart != nil {
		sets = append(sets, fmt.Sprintf("date_range_start = $%d", idx))
		args = append(args, *dateRangeStart)
		idx++
	}

	if dateRangeEnd != nil {
		sets = append(sets, fmt.Sprintf("date_range_end = $%d", idx))
		args = append(args, *dateRangeEnd)
		idx++
	}

	if expiresAt != nil {
		sets = append(sets, fmt.Sprintf("expires_at = $%d", idx))
		args = append(args, *expiresAt)
		idx++
	}

	if channels != nil {
		sets = append(sets, fmt.Sprintf("notification_channels = $%d", idx))
		args = append(args, channels)
		idx++
	}

	if len(sets) == 0 {
		return s.GetAlertByID(ctx, id)
	}

	sets = append(sets, "updated_at = now()")

	q := fmt.Sprintf(`
UPDATE appointment_alerts
SET %s
WHERE id = $1 AND deleted_at IS NULL
RETURNING id, user_id, destination_space_id, embassy_slug, visa_category,
          date_range_start, date_range_end, status, last_checked_at, fired_at,
          expires_at, notification_channels, created_at, updated_at, deleted_at
`, strings.Join(sets, ", "))

	var out AlertRow

	err := s.pool.Writer.QueryRow(ctx, q, args...).Scan(
		&out.ID, &out.UserID, &out.DestinationSpaceID,
		&out.EmbassySlug, &out.VisaCategory, &out.DateRangeStart, &out.DateRangeEnd,
		&out.Status, &out.LastCheckedAt, &out.FiredAt, &out.ExpiresAt,
		&out.NotificationChannels, &out.CreatedAt, &out.UpdatedAt, &out.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AlertRow{}, pgx.ErrNoRows
		}

		return AlertRow{}, fmt.Errorf("appointmentalert: update alert: %w", err)
	}

	return out, nil
}

// DeleteAlert soft-deletes an appointment_alerts row (Rule 11 — no hard
// delete of user content in V1 app code; only account:deletion_cleanup may
// hard-delete). Returns pgx.ErrNoRows if the row does not exist, is already
// soft-deleted, or belongs to a different platform.
func (s *Store) DeleteAlert(ctx context.Context, id pgtype.UUID) error {
	const q = `
UPDATE appointment_alerts
SET deleted_at = now(), updated_at = now(), status = 'expired'
WHERE id = $1 AND deleted_at IS NULL
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("appointmentalert: delete alert: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ExpireAlerts transitions all active appointment_alerts whose expires_at is
// in the past (or whose date_range_end is in the past when expires_at is
// NULL) from 'active' to 'expired'. Returns the number of rows transitioned.
// Called by the embassy:expire_checks Asynq nightly sweep (§91, ADR-099).
func (s *Store) ExpireAlerts(ctx context.Context) (int64, error) {
	const q = `
UPDATE appointment_alerts
SET status = 'expired', updated_at = now()
WHERE deleted_at IS NULL
  AND status = 'active'
  AND (
      (expires_at IS NOT NULL AND expires_at < CURRENT_DATE)
      OR
      (expires_at IS NULL AND date_range_end IS NOT NULL AND date_range_end < CURRENT_DATE)
  )
`

	cmd, err := s.pool.Writer.Exec(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("appointmentalert: expire alerts: %w", err)
	}

	return cmd.RowsAffected(), nil
}

// ListActiveAlertsByEmbassySlug returns all active (non-deleted, status =
// 'active') appointment_alerts for a given embassy_slug within a platform.
// Used by DispatchAlerts to notify users when availability is detected
// (§91, ADR-099). Only alerts whose date range covers the current date
// are returned.
func (s *Store) ListActiveAlertsByEmbassySlug(ctx context.Context, embassySlug string) ([]AlertRow, error) {
	const q = `
SELECT a.id, a.user_id, a.destination_space_id, sp.name AS dest_space_name,
       a.embassy_slug, a.visa_category, a.date_range_start, a.date_range_end,
       a.status, a.last_checked_at, a.fired_at, a.expires_at,
       a.notification_channels, a.created_at, a.updated_at, a.deleted_at
FROM appointment_alerts a
LEFT JOIN spaces sp ON sp.id = a.destination_space_id
WHERE a.embassy_slug = $1
  AND a.status = 'active'
  AND a.deleted_at IS NULL
  AND CURRENT_DATE >= a.date_range_start
  AND CURRENT_DATE <= a.date_range_end
ORDER BY a.created_at ASC
`

	rows, err := s.pool.Reader.Query(ctx, q, embassySlug)
	if err != nil {
		return nil, fmt.Errorf("appointmentalert: list active alerts by embassy slug: %w", err)
	}

	defer rows.Close()

	var out []AlertRow

	for rows.Next() {
		var r AlertRow
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.DestinationSpaceID, &r.DestSpaceName,
			&r.EmbassySlug, &r.VisaCategory, &r.DateRangeStart, &r.DateRangeEnd,
			&r.Status, &r.LastCheckedAt, &r.FiredAt, &r.ExpiresAt,
			&r.NotificationChannels, &r.CreatedAt, &r.UpdatedAt, &r.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("appointmentalert: scan alert row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("appointmentalert: iterate alert rows: %w", err)
	}

	return out, nil
}

// MarkAlertFired sets fired_at = now() and status = 'fired' on an active
// appointment_alert. The 24-hour pause-after-fire (ADR-099) is enforced by
// the caller: DispatchAlerts only re-fires an alert if fired_at is NULL or
// older than 24 hours. Returns pgx.ErrNoRows if the alert is not active.
func (s *Store) MarkAlertFired(ctx context.Context, id pgtype.UUID) error {
	const q = `
UPDATE appointment_alerts
SET fired_at = now(), status = 'fired', last_checked_at = now(), updated_at = now()
WHERE id = $1 AND status = 'active' AND deleted_at IS NULL
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("appointmentalert: mark alert fired: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ListEmbassiesDueForPoll returns all active monitored_embassies whose
// effective poll interval has elapsed since last_polled_at (or
// last_polled_at is NULL). Used by the recurring poll scheduler to enqueue
// embassy:poll tasks at each embassy's configured interval (§91, ADR-099).
// // The effective interval is computed per-embassy: if at least one
// premium-tier user (holding the appointment_alert.premium permission) has
// an active alert for the embassy, the effective interval is
// LEAST(e.poll_interval_seconds, premiumIntervalSeconds); otherwise it is
// e.poll_interval_seconds. This gives premium users faster polling without
// per-user poll tasks (§91, ADR-099).
func (s *Store) ListEmbassiesDueForPoll(ctx context.Context, premiumIntervalSeconds int32) ([]EmbassyRow, error) {
	q := `
WITH premium_embassies AS (
    -- Embassies that have at least one active alert from a premium-tier user.
    -- The RBAC chain: appointment_alerts → user_group_memberships →
    -- user_groups → group_roles → role_permissions → permissions.
    SELECT DISTINCT a.embassy_slug
    FROM appointment_alerts a
    JOIN user_group_memberships m ON m.user_id = a.user_id
    JOIN user_groups ug ON ug.id = m.group_id
    JOIN group_roles gr ON gr.group_id = ug.id
    JOIN role_permissions rp ON rp.role_id = gr.role_id
    JOIN permissions p ON p.id = rp.permission_id
    WHERE a.status = 'active'
      AND a.deleted_at IS NULL
      AND p.key = 'appointment_alert.premium'
)
SELECT e.id, e.destination_space_id, sp.name AS dest_space_name,
       e.slug, e.name, e.booking_url, e.poll_interval_seconds, e.is_active,
       e.last_polled_at, e.created_at, e.updated_at
FROM monitored_embassies e
LEFT JOIN spaces sp ON sp.id = e.destination_space_id
WHERE e.is_active = true
  AND (
      e.last_polled_at IS NULL
      OR e.last_polled_at + (
          (CASE
              WHEN EXISTS (
                  SELECT 1 FROM premium_embassies pe
                  WHERE pe.embassy_slug = e.slug
              )
              THEN LEAST(e.poll_interval_seconds, $1)
              ELSE e.poll_interval_seconds
          END) || ' seconds'
          )::interval <= now()
  )
ORDER BY e.last_polled_at ASC NULLS FIRST
`

	rows, err := s.pool.Reader.Query(ctx, q, premiumIntervalSeconds)
	if err != nil {
		return nil, fmt.Errorf("appointmentalert: list embassies due for poll: %w", err)
	}

	defer rows.Close()

	var out []EmbassyRow

	for rows.Next() {
		var r EmbassyRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID, &r.DestSpaceName,
			&r.Slug, &r.Name, &r.BookingURL, &r.PollIntervalSeconds, &r.IsActive,
			&r.LastPolledAt, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("appointmentalert: scan embassy row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("appointmentalert: iterate embassy rows: %w", err)
	}

	return out, nil
}

// GetEmbassyByID returns a single monitored embassy by its PK within the
// platform. Used by PollEmbassy to load the booking_url and slug.
func (s *Store) GetEmbassyByID(ctx context.Context, id pgtype.UUID) (EmbassyRow, error) {
	return s.getEmbassyByID(ctx, id)
}
