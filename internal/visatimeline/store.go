// Package visatimeline implements the Visa Timeline Tracker (§83, ADR-091).
//
// Members record their visa application journey as a timeline of milestones
// (application submitted, biometrics, interview, decision, etc.). Aggregated,
// anonymised statistics feed a public processing-time dashboard so others can
// gauge realistic timelines for a given origin → destination → visa category
// → consulate combination. Each member controls the visibility (shared vs
// private) of their own timeline.
//
// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md
//      japaconnect-engineering/adr/ADR-091-visa-timeline-tracker.md
package visatimeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for visa_timelines, visa_timeline_milestones,
// and visa_processing_stats. It uses direct pgx queries because these are new
// tables without sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a visa-timeline store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// Timeline holds a single visa_timelines row.
type Timeline struct {
	ID                pgtype.UUID
	TenantID          pgtype.UUID
	UserID            pgtype.UUID
	OriginSpaceID     pgtype.UUID
	DestinationSpaceID pgtype.UUID
	VisaCategory      string
	Consulate         *string
	ApplicationMethod *string
	Outcome           *string
	IsShared          bool
	AnonymisedAt      pgtype.Timestamptz
	CreatedAt         pgtype.Timestamptz
	UpdatedAt         pgtype.Timestamptz
	DeletedAt         pgtype.Timestamptz
}

// Milestone holds a single visa_timeline_milestones row.
type Milestone struct {
	ID            pgtype.UUID
	TenantID      pgtype.UUID
	TimelineID    pgtype.UUID
	MilestoneType string
	OccurredAt    pgtype.Timestamptz
	Notes         *string
	CreatedAt     pgtype.Timestamptz
}

// ProcessingStat holds a single visa_processing_stats row.
type ProcessingStat struct {
	ID                pgtype.UUID
	TenantID          pgtype.UUID
	OriginSpaceID     pgtype.UUID
	DestinationSpaceID pgtype.UUID
	VisaCategory      string
	Consulate         *string
	PeriodStart       pgtype.Timestamptz
	SampleCount       int32
	AvgDays           *float64
	MedianDays        *float64
	P10Days           *float64
	P90Days           *float64
	RefreshedAt       pgtype.Timestamptz
}

// TrendPoint is one month in the monthly trend series.
type TrendPoint struct {
	PeriodStart  pgtype.Timestamptz
	SampleCount  int32
	AvgDays      *float64
	MedianDays   *float64
}

// CreateTimeline inserts a new visa_timelines row and returns the created row.
func (s *Store) CreateTimeline(ctx context.Context, t Timeline) (Timeline, error) {
	const q = `
INSERT INTO visa_timelines (
    tenant_id, user_id, origin_space_id, destination_space_id,
    visa_category, consulate, application_method, outcome, is_shared
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, tenant_id, user_id, origin_space_id, destination_space_id,
    visa_category, consulate, application_method, outcome, is_shared,
    anonymised_at, created_at, updated_at, deleted_at
`
	var out Timeline
	err := s.pool.Writer.QueryRow(ctx, q,
		t.TenantID, t.UserID, t.OriginSpaceID, t.DestinationSpaceID,
		t.VisaCategory, t.Consulate, t.ApplicationMethod, t.Outcome, t.IsShared,
	).Scan(
		&out.ID, &out.TenantID, &out.UserID, &out.OriginSpaceID, &out.DestinationSpaceID,
		&out.VisaCategory, &out.Consulate, &out.ApplicationMethod, &out.Outcome, &out.IsShared,
		&out.AnonymisedAt, &out.CreatedAt, &out.UpdatedAt, &out.DeletedAt,
	)
	if err != nil {
		return Timeline{}, fmt.Errorf("visatimeline: create timeline: %w", err)
	}
	return out, nil
}

// GetTimeline retrieves a single non-deleted timeline by id within a tenant.
func (s *Store) GetTimeline(ctx context.Context, id, tenantID pgtype.UUID) (Timeline, error) {
	const q = `
SELECT id, tenant_id, user_id, origin_space_id, destination_space_id,
       visa_category, consulate, application_method, outcome, is_shared,
       anonymised_at, created_at, updated_at, deleted_at
FROM visa_timelines
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
`
	var t Timeline
	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&t.ID, &t.TenantID, &t.UserID, &t.OriginSpaceID, &t.DestinationSpaceID,
		&t.VisaCategory, &t.Consulate, &t.ApplicationMethod, &t.Outcome, &t.IsShared,
		&t.AnonymisedAt, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Timeline{}, pgx.ErrNoRows
		}
		return Timeline{}, fmt.Errorf("visatimeline: get timeline: %w", err)
	}
	return t, nil
}

// ListUserTimelines returns all non-deleted timelines owned by a user within
// a tenant, newest first.
func (s *Store) ListUserTimelines(ctx context.Context, userID, tenantID pgtype.UUID) ([]Timeline, error) {
	const q = `
SELECT id, tenant_id, user_id, origin_space_id, destination_space_id,
       visa_category, consulate, application_method, outcome, is_shared,
       anonymised_at, created_at, updated_at, deleted_at
FROM visa_timelines
WHERE user_id = $1 AND tenant_id = $2 AND deleted_at IS NULL
ORDER BY created_at DESC
`
	rows, err := s.pool.Reader.Query(ctx, q, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: list user timelines: %w", err)
	}
	defer rows.Close()

	var out []Timeline
	for rows.Next() {
		var t Timeline
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.UserID, &t.OriginSpaceID, &t.DestinationSpaceID,
			&t.VisaCategory, &t.Consulate, &t.ApplicationMethod, &t.Outcome, &t.IsShared,
			&t.AnonymisedAt, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("visatimeline: scan timeline: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("visatimeline: iterate timelines: %w", err)
	}
	return out, nil
}

// StoreUpdateTimelineInput holds the mutable fields for a timeline update at
// the store layer. Only non-nil fields are applied.
type StoreUpdateTimelineInput struct {
	Outcome           *string
	IsShared          *bool
	Consulate         *string
	ApplicationMethod *string
}

// UpdateTimeline updates the outcome/share toggle (and optional consulate /
// application method) on a non-deleted timeline within a tenant.
func (s *Store) UpdateTimeline(ctx context.Context, id, tenantID pgtype.UUID, input StoreUpdateTimelineInput) error {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if input.Outcome != nil {
		sets = append(sets, fmt.Sprintf("outcome = $%d", argIdx))
		args = append(args, *input.Outcome)
		argIdx++
	}
	if input.IsShared != nil {
		sets = append(sets, fmt.Sprintf("is_shared = $%d", argIdx))
		args = append(args, *input.IsShared)
		argIdx++
	}
	if input.Consulate != nil {
		sets = append(sets, fmt.Sprintf("consulate = $%d", argIdx))
		args = append(args, *input.Consulate)
		argIdx++
	}
	if input.ApplicationMethod != nil {
		sets = append(sets, fmt.Sprintf("application_method = $%d", argIdx))
		args = append(args, *input.ApplicationMethod)
		argIdx++
	}

	if len(sets) == 0 {
		return nil // nothing to update
	}

	sets = append(sets, "updated_at = now()")
	args = append(args, id, tenantID)

	q := fmt.Sprintf(`
UPDATE visa_timelines SET %s WHERE id = $%d AND tenant_id = $%d AND deleted_at IS NULL
`, strings.Join(sets, ", "), argIdx, argIdx+1)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("visatimeline: update timeline: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteTimeline soft-deletes a timeline by stamping deleted_at. The caller
// must verify ownership before calling.
func (s *Store) DeleteTimeline(ctx context.Context, id, tenantID pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `
UPDATE visa_timelines SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
`, id, tenantID)
	if err != nil {
		return fmt.Errorf("visatimeline: delete timeline: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// AddMilestone inserts a new milestone for a timeline and returns the created
// row.
func (s *Store) AddMilestone(ctx context.Context, timelineID, tenantID pgtype.UUID, milestoneType string, occurredAt pgtype.Timestamptz, notes *string) (Milestone, error) {
	const q = `
INSERT INTO visa_timeline_milestones (tenant_id, timeline_id, milestone_type, occurred_at, notes)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, timeline_id, milestone_type, occurred_at, notes, created_at
`
	var m Milestone
	err := s.pool.Writer.QueryRow(ctx, q,
		tenantID, timelineID, milestoneType, occurredAt, notes,
	).Scan(
		&m.ID, &m.TenantID, &m.TimelineID, &m.MilestoneType, &m.OccurredAt, &m.Notes, &m.CreatedAt,
	)
	if err != nil {
		return Milestone{}, fmt.Errorf("visatimeline: add milestone: %w", err)
	}
	return m, nil
}

// StoreUpdateMilestoneInput holds the mutable fields for a milestone at the
// store layer. Only non-nil fields are applied.
type StoreUpdateMilestoneInput struct {
	MilestoneType *string
	OccurredAt    *pgtype.Timestamptz
	Notes         *string
}

// UpdateMilestone edits a milestone within a tenant.
func (s *Store) UpdateMilestone(ctx context.Context, milestoneID, tenantID pgtype.UUID, input StoreUpdateMilestoneInput) error {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if input.MilestoneType != nil {
		sets = append(sets, fmt.Sprintf("milestone_type = $%d", argIdx))
		args = append(args, *input.MilestoneType)
		argIdx++
	}
	if input.OccurredAt != nil {
		sets = append(sets, fmt.Sprintf("occurred_at = $%d", argIdx))
		args = append(args, *input.OccurredAt)
		argIdx++
	}
	if input.Notes != nil {
		sets = append(sets, fmt.Sprintf("notes = $%d", argIdx))
		args = append(args, *input.Notes)
		argIdx++
	}

	if len(sets) == 0 {
		return nil // nothing to update
	}

	args = append(args, milestoneID, tenantID)

	q := fmt.Sprintf(`
UPDATE visa_timeline_milestones SET %s WHERE id = $%d AND tenant_id = $%d
`, strings.Join(sets, ", "), argIdx, argIdx+1)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("visatimeline: update milestone: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ListMilestones returns the milestones for a timeline, ordered chronologically.
func (s *Store) ListMilestones(ctx context.Context, timelineID, tenantID pgtype.UUID) ([]Milestone, error) {
	const q = `
SELECT id, tenant_id, timeline_id, milestone_type, occurred_at, notes, created_at
FROM visa_timeline_milestones
WHERE timeline_id = $1 AND tenant_id = $2
ORDER BY occurred_at ASC, created_at ASC
`
	rows, err := s.pool.Reader.Query(ctx, q, timelineID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: list milestones: %w", err)
	}
	defer rows.Close()

	var out []Milestone
	for rows.Next() {
		var m Milestone
		if err := rows.Scan(
			&m.ID, &m.TenantID, &m.TimelineID, &m.MilestoneType, &m.OccurredAt, &m.Notes, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("visatimeline: scan milestone: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("visatimeline: iterate milestones: %w", err)
	}
	return out, nil
}

// GetStats returns the aggregated processing-time statistics for a given
// origin → destination → visa category combination (consulate optional). The
// stats are read from the pre-aggregated visa_processing_stats table.
func (s *Store) GetStats(ctx context.Context, tenantID, originSpaceID, destSpaceID pgtype.UUID, visaCategory string) ([]ProcessingStat, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, tenantID)
	argIdx++

	if originSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("origin_space_id = $%d", argIdx))
		args = append(args, originSpaceID)
		argIdx++
	}
	if destSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, destSpaceID)
		argIdx++
	}
	if visaCategory != "" {
		conditions = append(conditions, fmt.Sprintf("visa_category = $%d", argIdx))
		args = append(args, visaCategory)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	q := fmt.Sprintf(`
SELECT id, tenant_id, origin_space_id, destination_space_id, visa_category,
       consulate, period_start, sample_count, avg_days, median_days,
       p10_days, p90_days, refreshed_at
FROM visa_processing_stats
WHERE %s
ORDER BY period_start DESC
`, whereClause)

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: get stats: %w", err)
	}
	defer rows.Close()

	var out []ProcessingStat
	for rows.Next() {
		var st ProcessingStat
		if err := rows.Scan(
			&st.ID, &st.TenantID, &st.OriginSpaceID, &st.DestinationSpaceID,
			&st.VisaCategory, &st.Consulate, &st.PeriodStart, &st.SampleCount,
			&st.AvgDays, &st.MedianDays, &st.P10Days, &st.P90Days, &st.RefreshedAt,
		); err != nil {
			return nil, fmt.Errorf("visatimeline: scan stat: %w", err)
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("visatimeline: iterate stats: %w", err)
	}
	return out, nil
}

// GetTrend returns a monthly trend series (avg/median processing days) for a
// destination space + visa category over the last `months` months.
func (s *Store) GetTrend(ctx context.Context, tenantID, destSpaceID pgtype.UUID, visaCategory string, months int) ([]TrendPoint, error) {
	if months <= 0 {
		months = 12
	}

	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, tenantID)
	argIdx++

	if destSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, destSpaceID)
		argIdx++
	}
	if visaCategory != "" {
		conditions = append(conditions, fmt.Sprintf("visa_category = $%d", argIdx))
		args = append(args, visaCategory)
		argIdx++
	}

	conditions = append(conditions, fmt.Sprintf("period_start >= now() - interval '%d months'", months))

	whereClause := strings.Join(conditions, " AND ")

	q := fmt.Sprintf(`
SELECT period_start, sample_count, avg_days, median_days
FROM visa_processing_stats
WHERE %s
ORDER BY period_start ASC
`, whereClause)

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: get trend: %w", err)
	}
	defer rows.Close()

	var out []TrendPoint
	for rows.Next() {
		var p TrendPoint
		if err := rows.Scan(&p.PeriodStart, &p.SampleCount, &p.AvgDays, &p.MedianDays); err != nil {
			return nil, fmt.Errorf("visatimeline: scan trend point: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("visatimeline: iterate trend: %w", err)
	}
	return out, nil
}

// AdminListFilter holds filter parameters for the admin moderation list.
type AdminListFilter struct {
	TenantID          pgtype.UUID
	UserID            pgtype.UUID // optional: filter by submitter
	DestinationSpaceID pgtype.UUID // optional
	VisaCategory      string      // optional
	Outcome           string      // optional
	Q                 string      // optional: search visa_category/consulate
	Limit             int32
	Offset            int32
}

// AdminList returns all timelines (any share state) for the moderation queue.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]Timeline, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	conditions = append(conditions, "deleted_at IS NULL")

	if f.UserID.Valid {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, f.UserID)
		argIdx++
	}
	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}
	if f.VisaCategory != "" {
		conditions = append(conditions, fmt.Sprintf("visa_category = $%d", argIdx))
		args = append(args, f.VisaCategory)
		argIdx++
	}
	if f.Outcome != "" {
		conditions = append(conditions, fmt.Sprintf("outcome = $%d", argIdx))
		args = append(args, f.Outcome)
		argIdx++
	}
	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("(visa_category ILIKE $%d OR consulate ILIKE $%d)", argIdx, argIdx))
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

	listQ := fmt.Sprintf(`
SELECT id, tenant_id, user_id, origin_space_id, destination_space_id,
       visa_category, consulate, application_method, outcome, is_shared,
       anonymised_at, created_at, updated_at, deleted_at
FROM visa_timelines
WHERE %s
ORDER BY created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("visatimeline: admin list: %w", err)
	}
	defer rows.Close()

	var out []Timeline
	for rows.Next() {
		var t Timeline
		if err := rows.Scan(
			&t.ID, &t.TenantID, &t.UserID, &t.OriginSpaceID, &t.DestinationSpaceID,
			&t.VisaCategory, &t.Consulate, &t.ApplicationMethod, &t.Outcome, &t.IsShared,
			&t.AnonymisedAt, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("visatimeline: admin scan timeline: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("visatimeline: admin iterate timelines: %w", err)
	}

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM visa_timelines WHERE %s`, whereClause)
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("visatimeline: admin count timelines: %w", err)
	}

	return out, total, nil
}
