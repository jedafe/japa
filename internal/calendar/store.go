// Package calendar implements the Community Calendar (§86, ADR-094).
//
// A community-driven event calendar for each destination Space. Members
// submit events (festivals, meetups, markets) which enter a pending review
// queue. Moderators approve, archive, or flag submissions. Authenticated
// users can save individual events and subscribe to category/destination
// notifications.
//
// BUG-007: fixed-shape queries are now sqlc-generated (Rule 2). The
// dynamic-filter queries (ListEntries, AdminListEntries,
// UpcomingEntriesForReminders) remain hand-written pgx because sqlc
// cannot express dynamic WHERE clauses or dynamic OR builders.
//
// Ref: japaconnect-engineering/sections/86-community-calendar.md
//
//	japaconnect-engineering/adr/ADR-094-community-calendar.md
package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store is the data-access layer for calendar_entries, calendar_saves,
// and calendar_subscriptions. Fixed-shape queries use sqlc-generated
// methods (read/write split); dynamic-filter queries use hand-written
// pgx on the raw pool.
type Store struct {
	pool  *db.DB
	read  *gen.Queries
	write *gen.Queries
}

// NewStore creates a calendar store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{
		pool:  database,
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
	}
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
	SourceURL          string // NOT NULL since 000378
	Status             string // pending|published|archived|unverified (§86)
	SubmittedBy        pgtype.UUID
	ReviewedBy         pgtype.UUID
	ReviewedAt         pgtype.Timestamptz
	StalenessCheckDue  pgtype.Timestamptz
	// ADR-088 universal AI content generation columns.
	AiGenerated    bool
	AiModel        *string
	HitlReviewedBy pgtype.UUID
	AiGeneratedAt  pgtype.Timestamptz
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
	DeletedAt      pgtype.Timestamptz
}

// fromGetRow converts a sqlc-generated CalendarGetEntryRow to CalendarEntry.
func fromGetRow(r gen.CalendarGetEntryRow) CalendarEntry {
	return CalendarEntry{
		ID: r.ID, TenantID: r.TenantID, Title: r.Title, Description: r.Description,
		Category: r.Category, DestinationSpaceID: r.DestinationSpaceID, OriginSpaceID: r.OriginSpaceID,
		OccursAt: r.OccursAt, EndsAt: r.EndsAt, IsRecurring: r.IsRecurring, RecurrencePattern: r.RecurrencePattern,
		SourceURL: r.SourceUrl, Status: r.Status, SubmittedBy: r.SubmittedBy, ReviewedBy: r.ReviewedBy, ReviewedAt: r.ReviewedAt,
		StalenessCheckDue: r.StalenessCheckDue,
		AiGenerated:       r.AiGenerated, AiModel: r.AiModel, HitlReviewedBy: r.HitlReviewedBy, AiGeneratedAt: r.AiGeneratedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt,
	}
}

// fromSavedRow converts a sqlc-generated CalendarListSavedEntriesRow to CalendarEntry.
func fromSavedRow(r gen.CalendarListSavedEntriesRow) CalendarEntry {
	return CalendarEntry{
		ID: r.ID, TenantID: r.TenantID, Title: r.Title, Description: r.Description,
		Category: r.Category, DestinationSpaceID: r.DestinationSpaceID, OriginSpaceID: r.OriginSpaceID,
		OccursAt: r.OccursAt, EndsAt: r.EndsAt, IsRecurring: r.IsRecurring, RecurrencePattern: r.RecurrencePattern,
		SourceURL: r.SourceUrl, Status: r.Status, SubmittedBy: r.SubmittedBy, ReviewedBy: r.ReviewedBy, ReviewedAt: r.ReviewedAt,
		StalenessCheckDue: r.StalenessCheckDue,
		AiGenerated:       r.AiGenerated, AiModel: r.AiModel, HitlReviewedBy: r.HitlReviewedBy, AiGeneratedAt: r.AiGeneratedAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, DeletedAt: r.DeletedAt,
	}
}

// CreateEntry inserts a new calendar entry with status='pending'.
func (s *Store) CreateEntry(ctx context.Context, entry CalendarEntry) (pgtype.UUID, error) {
	id, err := s.write.CalendarCreateEntry(ctx, gen.CalendarCreateEntryParams{
		TenantID:           entry.TenantID,
		Title:              entry.Title,
		Description:        entry.Description,
		Category:           entry.Category,
		DestinationSpaceID: entry.DestinationSpaceID,
		OriginSpaceID:      entry.OriginSpaceID,
		OccursAt:           entry.OccursAt,
		EndsAt:             entry.EndsAt,
		IsRecurring:        entry.IsRecurring,
		RecurrencePattern:  entry.RecurrencePattern,
		SourceUrl:          entry.SourceURL,
		SubmittedBy:        entry.SubmittedBy,
		StalenessCheckDue:  entry.StalenessCheckDue,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("calendar: create entry: %w", err)
	}
	return id, nil
}

// SetStalenessCheckDue sets staleness_check_due = occurs_at - 60 days for
// recurring entries. Non-recurring entries get a NULL staleness_check_due.
// Ref: §86 Staleness Management, ADR-094.
func (s *Store) SetStalenessCheckDue(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.CalendarSetStalenessCheckDue(ctx, id); err != nil {
		return fmt.Errorf("calendar: set staleness_check_due: %w", err)
	}
	return nil
}

// FlagStaleEntries marks entries as 'unverified' when their
// staleness_check_due date has passed. Returns the count of flagged entries.
// Ref: §86 Staleness Management — nightly Asynq task calendar:staleness_check.
func (s *Store) FlagStaleEntries(ctx context.Context) (int64, error) {
	n, err := s.write.CalendarFlagStaleEntries(ctx)
	if err != nil {
		return 0, fmt.Errorf("calendar: flag stale entries: %w", err)
	}
	return n, nil
}

// UpcomingEntriesForReminders returns published entries occurring at the
// specified day-offset marks (e.g. 90/30/7 days from now) that have
// subscribers. Used by calendar:send_reminders.
// Ref: §86 — reminders fire at 90/30/7 days before each entry.
//
// BUG-012: the SQL pre-filters to a 1-day-wide window around each mark so
// the Go loop only processes entries that are actually due a reminder,
// instead of loading the entire 90-day window and skipping most rows.
// The window is [today + days, today + days + 1 day) so that a daily task
// firing at any time of day catches entries whose occurs_at falls on the
// target calendar day.
//
// This query uses a dynamic OR clause and cannot be expressed in sqlc.
func (s *Store) UpcomingEntriesForReminders(ctx context.Context, dayMarks []int) ([]CalendarEntry, error) {
	if len(dayMarks) == 0 {
		return nil, nil
	}
	// Build a parameterised OR clause: occurs_at in [now()+d days, now()+(d+1) days)
	// for each mark d. Args start at $1.
	args := make([]any, 0, len(dayMarks)*2)
	clauses := make([]string, 0, len(dayMarks))
	argIdx := 1
	for _, d := range dayMarks {
		clauses = append(clauses, fmt.Sprintf(
			"(e.occurs_at >= now() + make_interval(days => $%d) AND e.occurs_at < now() + make_interval(days => $%d))",
			argIdx, argIdx+1,
		))
		args = append(args, d, d+1)
		argIdx += 2
	}
	whereClause := strings.Join(clauses, " OR ")

	query := fmt.Sprintf(`
SELECT e.id, e.tenant_id, e.title, e.description, e.category,
       e.destination_space_id, e.origin_space_id,
       e.occurs_at, e.ends_at, e.is_recurring, e.recurrence_pattern,
       e.source_url, e.status, e.submitted_by, e.reviewed_by, e.reviewed_at,
       e.staleness_check_due,
       e.ai_generated, e.ai_model, e.hitl_reviewed_by, e.ai_generated_at,
       e.created_at, e.updated_at, e.deleted_at
FROM calendar_entries e
WHERE e.status = 'published'
  AND e.deleted_at IS NULL
  AND (%s)
ORDER BY e.occurs_at ASC
`, whereClause)

	rows, err := s.pool.Reader.Query(ctx, query, args...)
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
			&e.StalenessCheckDue,
			&e.AiGenerated, &e.AiModel, &e.HitlReviewedBy, &e.AiGeneratedAt,
			&e.CreatedAt, &e.UpdatedAt, &e.DeletedAt,
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
	catPtr := &entry.Category
	users, err := s.read.CalendarSubscribersForEntry(ctx, gen.CalendarSubscribersForEntryParams{
		TenantID:           entry.TenantID,
		DestinationSpaceID: entry.DestinationSpaceID,
		Category:           catPtr,
	})
	if err != nil {
		return nil, fmt.Errorf("calendar: subscribers for entry: %w", err)
	}
	return users, nil
}

// GetEntry returns a single published calendar entry by id and tenant.
func (s *Store) GetEntry(ctx context.Context, id, tenantID pgtype.UUID) (CalendarEntry, error) {
	row, err := s.read.CalendarGetEntry(ctx, gen.CalendarGetEntryParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return CalendarEntry{}, pgx.ErrNoRows
		}
		return CalendarEntry{}, fmt.Errorf("calendar: get entry: %w", err)
	}
	return fromGetRow(row), nil
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
//
// This query uses a dynamic WHERE clause and cannot be expressed in sqlc.
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
       staleness_check_due,
       ai_generated, ai_model, hitl_reviewed_by, ai_generated_at,
       created_at, updated_at, deleted_at,
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
			&e.StalenessCheckDue,
			&e.AiGenerated, &e.AiModel, &e.HitlReviewedBy, &e.AiGeneratedAt,
			&e.CreatedAt, &e.UpdatedAt, &e.DeletedAt,
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
	if err := s.write.CalendarSaveEntry(ctx, gen.CalendarSaveEntryParams{
		UserID: userID, EntryID: entryID, TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("calendar: save entry: %w", err)
	}
	return nil
}

// UnsaveEntry deletes a calendar_saves row.
func (s *Store) UnsaveEntry(ctx context.Context, userID, entryID, tenantID pgtype.UUID) error {
	if err := s.write.CalendarUnsaveEntry(ctx, gen.CalendarUnsaveEntryParams{
		UserID: userID, EntryID: entryID, TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("calendar: unsave entry: %w", err)
	}
	return nil
}

// ListSavedEntries returns the calendar entries saved by a given user,
// joined via calendar_saves. Used by GET /me/calendar/saved (§86 Integration:
// "Saved entries appear on the user dashboard alongside visa checklists.").
// Only published, non-deleted entries are returned.
func (s *Store) ListSavedEntries(ctx context.Context, userID, tenantID pgtype.UUID, limit, offset int) ([]CalendarEntry, int64, error) {
	rows, err := s.read.CalendarListSavedEntries(ctx, gen.CalendarListSavedEntriesParams{
		UserID:   userID,
		TenantID: tenantID,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("calendar: list saved entries: %w", err)
	}

	out := make([]CalendarEntry, 0, len(rows))
	var total int64
	for _, r := range rows {
		if total == 0 {
			total = r.TotalCount
		}
		out = append(out, fromSavedRow(r))
	}
	return out, total, nil
}

// Subscribe inserts a calendar_subscriptions row.
func (s *Store) Subscribe(ctx context.Context, userID, tenantID, destSpaceID pgtype.UUID, category string) error {
	catPtr := &category
	if err := s.write.CalendarSubscribe(ctx, gen.CalendarSubscribeParams{
		UserID:             userID,
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		Category:           catPtr,
	}); err != nil {
		return fmt.Errorf("calendar: subscribe: %w", err)
	}
	return nil
}

// Unsubscribe deletes a calendar_subscriptions row.
func (s *Store) Unsubscribe(ctx context.Context, userID, tenantID, destSpaceID pgtype.UUID, category string) error {
	catPtr := &category
	if err := s.write.CalendarUnsubscribe(ctx, gen.CalendarUnsubscribeParams{
		UserID:             userID,
		TenantID:           tenantID,
		DestinationSpaceID: destSpaceID,
		Category:           catPtr,
	}); err != nil {
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
//
// This query uses a dynamic WHERE clause + dynamic ORDER BY and cannot
// be expressed in sqlc.
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
       staleness_check_due,
       ai_generated, ai_model, hitl_reviewed_by, ai_generated_at,
       created_at, updated_at, deleted_at,
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
			&e.StalenessCheckDue,
			&e.AiGenerated, &e.AiModel, &e.HitlReviewedBy, &e.AiGeneratedAt,
			&e.CreatedAt, &e.UpdatedAt, &e.DeletedAt,
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
	n, err := s.write.CalendarAdminUpdateEntry(ctx, gen.CalendarAdminUpdateEntryParams{
		ID:         id,
		TenantID:   tenantID,
		Status:     status,
		ReviewedBy: reviewerID,
	})
	if err != nil {
		return fmt.Errorf("calendar: admin update entry: %w", err)
	}
	if n == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// AdminSoftDeleteEntry soft-deletes a calendar entry by setting deleted_at.
// Per Rule 11, user content (community-submitted entries) must be soft-deleted,
// never hard-deleted. The entry remains in the DB for audit/GDPR purposes but
// is excluded from all list/get queries (which filter deleted_at IS NULL).
// Ref: japaconnect-engineering/sections/86-community-calendar.md
// Ref: japaconnect-engineering/AGENTS.md Rule 11
func (s *Store) AdminSoftDeleteEntry(ctx context.Context, id, tenantID pgtype.UUID) error {
	n, err := s.write.CalendarAdminSoftDeleteEntry(ctx, gen.CalendarAdminSoftDeleteEntryParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("calendar: admin soft delete entry: %w", err)
	}
	if n == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
