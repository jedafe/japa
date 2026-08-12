// Package calendar implements the Community Calendar (§86, ADR-094).
//
// A community-driven event calendar for each destination Space. Members
// submit events (festivals, meetups, markets) which enter a pending review
// queue. Moderators approve, archive, or flag submissions. Authenticated
// users can save individual events and subscribe to category/destination
// notifications.
//
// Ref: japaconnect-engineering/sections/86-community-calendar.md
//      japaconnect-engineering/adr/ADR-094-community-calendar.md
package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for calendar_entries, calendar_saves,
// and calendar_subscriptions.
type Store struct {
	pool *db.DB
}

// NewStore creates a calendar store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// CalendarEntry represents a single calendar_entries row.
type CalendarEntry struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	Title              string
	Description        *string
	Category           string
	DestinationSpaceID pgtype.UUID
	OriginSpaceID      pgtype.UUID
	OccursAt           pgtype.Timestamptz
	EndsAt             pgtype.Timestamptz
	IsRecurring        bool
	RecurrencePattern  *string
	SourceURL          *string
	Status             string // pending|published|archived|unverified (§86)
	SubmittedBy        pgtype.UUID
	ReviewedBy         pgtype.UUID
	ReviewedAt         pgtype.Timestamptz
	StalenessCheckDue  pgtype.Timestamptz
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
	DeletedAt          pgtype.Timestamptz
}

// CreateEntry inserts a new calendar entry with status='pending'.
func (s *Store) CreateEntry(ctx context.Context, entry CalendarEntry) (pgtype.UUID, error) {
	var entryID pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
INSERT INTO calendar_entries (
    tenant_id, title, description, category,
    destination_space_id, origin_space_id,
    occurs_at, ends_at, is_recurring, recurrence_pattern,
    source_url, status, submitted_by, staleness_check_due
) VALUES (
    $1, $2, $3, $4,
    $5, $6,
    $7, $8, $9, $10,
    $11, 'pending', $12, $13
)
RETURNING id
`,
		entry.TenantID, entry.Title, entry.Description, entry.Category,
		entry.DestinationSpaceID, entry.OriginSpaceID,
		entry.OccursAt, entry.EndsAt, entry.IsRecurring, entry.RecurrencePattern,
		entry.SourceURL, entry.SubmittedBy, entry.StalenessCheckDue,
	).Scan(&entryID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("calendar: create entry: %w", err)
	}
	return entryID, nil
}

// SetStalenessCheckDue sets staleness_check_due = occurs_at - 60 days for
// recurring entries. Non-recurring entries get a NULL staleness_check_due.
// Ref: §86 Staleness Management, ADR-094.
func (s *Store) SetStalenessCheckDue(ctx context.Context, id pgtype.UUID) error {
	_, err := s.pool.Writer.Exec(ctx, `
UPDATE calendar_entries
SET staleness_check_due = CASE
    WHEN is_recurring THEN occurs_at - INTERVAL '60 days'
    ELSE NULL
END
WHERE id = $1
`, id)
	if err != nil {
		return fmt.Errorf("calendar: set staleness_check_due: %w", err)
	}
	return nil
}

// FlagStaleEntries marks entries as 'unverified' when their
// staleness_check_due date has passed. Returns the count of flagged entries.
// Ref: §86 Staleness Management — nightly Asynq task calendar:staleness_check.
func (s *Store) FlagStaleEntries(ctx context.Context) (int64, error) {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE calendar_entries
SET status = 'unverified', updated_at = now()
WHERE staleness_check_due IS NOT NULL
  AND staleness_check_due <= now()
  AND status = 'published'
  AND deleted_at IS NULL
`)
	if err != nil {
		return 0, fmt.Errorf("calendar: flag stale entries: %w", err)
	}
	return cmd.RowsAffected(), nil
}

// UpcomingEntriesForReminders returns published entries occurring within the
// next `daysAhead` days that have subscribers. Used by calendar:send_reminders.
// Ref: §86 — reminders fire at 90/30/7 days before each entry.
func (s *Store) UpcomingEntriesForReminders(ctx context.Context, daysAhead int) ([]CalendarEntry, error) {
	rows, err := s.pool.Reader.Query(ctx, `
SELECT e.id, e.tenant_id, e.title, e.description, e.category,
       e.destination_space_id, e.origin_space_id,
       e.occurs_at, e.ends_at, e.is_recurring, e.recurrence_pattern,
       e.source_url, e.status, e.submitted_by, e.reviewed_by, e.reviewed_at,
       e.staleness_check_due, e.created_at, e.updated_at, e.deleted_at
FROM calendar_entries e
WHERE e.status = 'published'
  AND e.deleted_at IS NULL
  AND e.occurs_at >= now()
  AND e.occurs_at <= now() + make_interval(days => $1)
ORDER BY e.occurs_at ASC
`, daysAhead)
	if err != nil {
		return nil, fmt.Errorf("calendar: upcoming entries: %w", err)
	}
	defer rows.Close()

	var out []CalendarEntry
	for rows.Next() {
		var e CalendarEntry
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.Title, &e.Description, &e.Category,
			&e.DestinationSpaceID, &e.OriginSpaceID,
			&e.OccursAt, &e.EndsAt, &e.IsRecurring, &e.RecurrencePattern,
			&e.SourceURL, &e.Status, &e.SubmittedBy, &e.ReviewedBy, &e.ReviewedAt,
			&e.StalenessCheckDue, &e.CreatedAt, &e.UpdatedAt, &e.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("calendar: scan upcoming entry: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("calendar: iterate upcoming entries: %w", err)
	}
	return out, nil
}

// SubscribersForEntry returns user IDs subscribed to the entry's
// destination+category combination.
func (s *Store) SubscribersForEntry(ctx context.Context, entry CalendarEntry) ([]pgtype.UUID, error) {
	rows, err := s.pool.Reader.Query(ctx, `
SELECT user_id FROM calendar_subscriptions
WHERE tenant_id = $1
  AND destination_space_id = $2
  AND (category IS NULL OR category = $3 OR category = '')
`, entry.TenantID, entry.DestinationSpaceID, entry.Category)
	if err != nil {
		return nil, fmt.Errorf("calendar: subscribers for entry: %w", err)
	}
	defer rows.Close()

	var out []pgtype.UUID
	for rows.Next() {
		var uid pgtype.UUID
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("calendar: scan subscriber: %w", err)
		}
		out = append(out, uid)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("calendar: iterate subscribers: %w", err)
	}
	return out, nil
}

// GetEntry returns a single published calendar entry by id and tenant.
func (s *Store) GetEntry(ctx context.Context, id, tenantID pgtype.UUID) (CalendarEntry, error) {
	const q = `
SELECT id, tenant_id, title, description, category,
       destination_space_id, origin_space_id,
       occurs_at, ends_at, is_recurring, recurrence_pattern,
       source_url, status, submitted_by, reviewed_by, reviewed_at,
       staleness_check_due, created_at, updated_at, deleted_at
FROM calendar_entries
WHERE id = $1 AND tenant_id = $2 AND status = 'published'
  AND deleted_at IS NULL
LIMIT 1
`
	var e CalendarEntry
	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&e.ID, &e.TenantID, &e.Title, &e.Description, &e.Category,
		&e.DestinationSpaceID, &e.OriginSpaceID,
		&e.OccursAt, &e.EndsAt, &e.IsRecurring, &e.RecurrencePattern,
		&e.SourceURL, &e.Status, &e.SubmittedBy, &e.ReviewedBy, &e.ReviewedAt,
		&e.StalenessCheckDue, &e.CreatedAt, &e.UpdatedAt, &e.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return CalendarEntry{}, pgx.ErrNoRows
		}
		return CalendarEntry{}, fmt.Errorf("calendar: get entry: %w", err)
	}
	return e, nil
}

// ListEntriesFilter holds filter parameters for the public entry list.
type ListEntriesFilter struct {
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID
	OriginSpaceID      pgtype.UUID
	Category           string
	From               pgtype.Timestamptz
	To                 pgtype.Timestamptz
	Limit              int32
	Offset             int32
}

// ListEntries returns published calendar entries matching the filter.
// Uses COUNT(*) OVER() window function to return the total count in the
// same query, avoiding a separate COUNT query (BUG-024).
func (s *Store) ListEntries(ctx context.Context, f ListEntriesFilter) ([]CalendarEntry, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	conditions = append(conditions, "status = 'published'")
	conditions = append(conditions, "deleted_at IS NULL")

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}
	if f.OriginSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("origin_space_id = $%d", argIdx))
		args = append(args, f.OriginSpaceID)
		argIdx++
	}
	if f.Category != "" {
		conditions = append(conditions, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, f.Category)
		argIdx++
	}
	if f.From.Valid {
		conditions = append(conditions, fmt.Sprintf("occurs_at >= $%d", argIdx))
		args = append(args, f.From)
		argIdx++
	}
	if f.To.Valid {
		conditions = append(conditions, fmt.Sprintf("occurs_at <= $%d", argIdx))
		args = append(args, f.To)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// BUG-024: use COUNT(*) OVER() to get total in the same query,
	// eliminating the separate COUNT query that doubled query cost.
	listQ := fmt.Sprintf(`
SELECT id, tenant_id, title, description, category,
       destination_space_id, origin_space_id,
       occurs_at, ends_at, is_recurring, recurrence_pattern,
       source_url, status, submitted_by, reviewed_by, reviewed_at,
       staleness_check_due, created_at, updated_at, deleted_at,
       COUNT(*) OVER() AS total_count
FROM calendar_entries
WHERE %s
ORDER BY occurs_at ASC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("calendar: list entries: %w", err)
	}
	defer rows.Close()

	var out []CalendarEntry
	var total int64
	for rows.Next() {
		var e CalendarEntry
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.Title, &e.Description, &e.Category,
			&e.DestinationSpaceID, &e.OriginSpaceID,
			&e.OccursAt, &e.EndsAt, &e.IsRecurring, &e.RecurrencePattern,
			&e.SourceURL, &e.Status, &e.SubmittedBy, &e.ReviewedBy, &e.ReviewedAt,
			&e.StalenessCheckDue, &e.CreatedAt, &e.UpdatedAt, &e.DeletedAt,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("calendar: scan entry: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("calendar: iterate entries: %w", err)
	}

	return out, total, nil
}

// SaveEntry inserts a calendar_saves row (user bookmarks an event).
func (s *Store) SaveEntry(ctx context.Context, userID, entryID, tenantID pgtype.UUID) error {
	_, err := s.pool.Writer.Exec(ctx, `
INSERT INTO calendar_saves (user_id, entry_id, tenant_id, saved_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (user_id, entry_id) DO NOTHING
`, userID, entryID, tenantID)
	if err != nil {
		return fmt.Errorf("calendar: save entry: %w", err)
	}
	return nil
}

// UnsaveEntry deletes a calendar_saves row.
func (s *Store) UnsaveEntry(ctx context.Context, userID, entryID, tenantID pgtype.UUID) error {
	_, err := s.pool.Writer.Exec(ctx, `
DELETE FROM calendar_saves
WHERE user_id = $1 AND entry_id = $2 AND tenant_id = $3
`, userID, entryID, tenantID)
	if err != nil {
		return fmt.Errorf("calendar: unsave entry: %w", err)
	}
	return nil
}

// Subscribe inserts a calendar_subscriptions row.
func (s *Store) Subscribe(ctx context.Context, userID, tenantID, destSpaceID pgtype.UUID, category string) error {
	_, err := s.pool.Writer.Exec(ctx, `
INSERT INTO calendar_subscriptions (user_id, tenant_id, destination_space_id, category, created_at)
VALUES ($1, $2, $3, $4, now())
ON CONFLICT (tenant_id, user_id, destination_space_id, category) DO NOTHING
`, userID, tenantID, destSpaceID, category)
	if err != nil {
		return fmt.Errorf("calendar: subscribe: %w", err)
	}
	return nil
}

// Unsubscribe deletes a calendar_subscriptions row.
func (s *Store) Unsubscribe(ctx context.Context, userID, tenantID, destSpaceID pgtype.UUID, category string) error {
	_, err := s.pool.Writer.Exec(ctx, `
DELETE FROM calendar_subscriptions
WHERE user_id = $1 AND tenant_id = $2 AND destination_space_id = $3 AND category = $4
`, userID, tenantID, destSpaceID, category)
	if err != nil {
		return fmt.Errorf("calendar: unsubscribe: %w", err)
	}
	return nil
}

// AdminListEntriesFilter holds filter parameters for the admin entry list.
type AdminListEntriesFilter struct {
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID
	OriginSpaceID      pgtype.UUID
	Category           string
	Status             string
	From               pgtype.Timestamptz
	To                 pgtype.Timestamptz
	Q                  string
	Sort               string
	Order              string
	Limit              int32
	Offset             int32
}

// AdminListEntries returns all calendar entries (any status) for admin
// management (paginated). Uses COUNT(*) OVER() window function to return
// the total count in the same query (BUG-024).
func (s *Store) AdminListEntries(ctx context.Context, f AdminListEntriesFilter) ([]CalendarEntry, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	conditions = append(conditions, "deleted_at IS NULL")

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}
	if f.OriginSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("origin_space_id = $%d", argIdx))
		args = append(args, f.OriginSpaceID)
		argIdx++
	}
	if f.Category != "" {
		conditions = append(conditions, fmt.Sprintf("category = $%d", argIdx))
		args = append(args, f.Category)
		argIdx++
	}
	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}
	if f.From.Valid {
		conditions = append(conditions, fmt.Sprintf("occurs_at >= $%d", argIdx))
		args = append(args, f.From)
		argIdx++
	}
	if f.To.Valid {
		conditions = append(conditions, fmt.Sprintf("occurs_at <= $%d", argIdx))
		args = append(args, f.To)
		argIdx++
	}
	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("title ILIKE $%d", argIdx))
		args = append(args, "%"+f.Q+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	// Sort: allow created_at, occurs_at, title, status. Default created_at DESC.
	sortCol := "created_at"
	switch f.Sort {
	case "occurs_at", "title", "status", "updated_at":
		sortCol = f.Sort
	}
	sortDir := "DESC"
	if f.Order == "asc" {
		sortDir = "ASC"
	}

	// BUG-024: use COUNT(*) OVER() to get total in the same query,
	// eliminating the separate COUNT query that doubled query cost.
	listQ := fmt.Sprintf(`
SELECT id, tenant_id, title, description, category,
       destination_space_id, origin_space_id,
       occurs_at, ends_at, is_recurring, recurrence_pattern,
       source_url, status, submitted_by, reviewed_by, reviewed_at,
       staleness_check_due, created_at, updated_at, deleted_at,
       COUNT(*) OVER() AS total_count
FROM calendar_entries
WHERE %s
ORDER BY %s %s
LIMIT $%d OFFSET $%d
`, whereClause, sortCol, sortDir, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("calendar: admin list entries: %w", err)
	}
	defer rows.Close()

	var out []CalendarEntry
	var total int64
	for rows.Next() {
		var e CalendarEntry
		if err := rows.Scan(
			&e.ID, &e.TenantID, &e.Title, &e.Description, &e.Category,
			&e.DestinationSpaceID, &e.OriginSpaceID,
			&e.OccursAt, &e.EndsAt, &e.IsRecurring, &e.RecurrencePattern,
			&e.SourceURL, &e.Status, &e.SubmittedBy, &e.ReviewedBy, &e.ReviewedAt,
			&e.StalenessCheckDue, &e.CreatedAt, &e.UpdatedAt, &e.DeletedAt,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("calendar: admin scan entry: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("calendar: admin iterate entries: %w", err)
	}

	return out, total, nil
}

// AdminUpdateEntry updates the status of a calendar entry and stamps the
// reviewer. Valid statuses: published, archived, unverified.
func (s *Store) AdminUpdateEntry(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, status string) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE calendar_entries
SET status = $3, reviewed_by = $4, reviewed_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
`, id, tenantID, status, reviewerID)
	if err != nil {
		return fmt.Errorf("calendar: admin update entry: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
