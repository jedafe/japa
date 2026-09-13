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
//
//	japaconnect-engineering/adr/ADR-091-visa-timeline-tracker.md
package visatimeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	UserID             pgtype.UUID
	OriginSpaceID      pgtype.UUID
	DestinationSpaceID pgtype.UUID
	VisaCategory       string
	Consulate          *string
	ApplicationMethod  *string
	Outcome            *string
	IsShared           bool
	AnonymisedAt       pgtype.Timestamptz
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
	DeletedAt          pgtype.Timestamptz
	// Milestones is populated by ListUserTimelines / AdminList via a
	// LATERAL join so the API can return timelines with their milestones
	// in a single round-trip (§83 — no separate list-milestones route).
	Milestones []Milestone
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
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	OriginSpaceID      pgtype.UUID
	DestinationSpaceID pgtype.UUID
	VisaCategory       string
	Consulate          *string
	PeriodStart        pgtype.Timestamptz
	SampleCount        int32
	AvgDays            *float64
	MedianDays         *float64
	P10Days            *float64
	P90Days            *float64
	RefreshedAt        pgtype.Timestamptz
}

// TrendPoint is one month in the monthly trend series.
type TrendPoint struct {
	PeriodStart pgtype.Timestamptz
	SampleCount int32
	AvgDays     *float64
	MedianDays  *float64
}

// CreateTimeline inserts a new visa_timelines row and returns the created row.
func (s *Store) CreateTimeline(ctx context.Context, t Timeline) (Timeline, error) {
	const q = `
INSERT INTO visa_timelines (
    user_id, origin_space_id, destination_space_id,
    visa_category, consulate, application_method, outcome, is_shared
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, user_id, origin_space_id, destination_space_id,
    visa_category, consulate, application_method, outcome, is_shared,
    anonymised_at, created_at, updated_at, deleted_at
`

	var out Timeline

	err := s.pool.Writer.QueryRow(
		ctx, q,
		t.UserID, t.OriginSpaceID, t.DestinationSpaceID,
		t.VisaCategory, t.Consulate, t.ApplicationMethod, t.Outcome, t.IsShared,
	).Scan(
		&out.ID, &out.UserID, &out.OriginSpaceID, &out.DestinationSpaceID,
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
SELECT id, user_id, origin_space_id, destination_space_id,
       visa_category, consulate, application_method, outcome, is_shared,
       anonymised_at, created_at, updated_at, deleted_at
FROM visa_timelines
WHERE id = $1 AND deleted_at IS NULL
`

	var t Timeline

	err := s.pool.Reader.QueryRow(ctx, q, id, tenantID).Scan(
		&t.ID, &t.UserID, &t.OriginSpaceID, &t.DestinationSpaceID,
		&t.VisaCategory, &t.Consulate, &t.ApplicationMethod, &t.Outcome, &t.IsShared,
		&t.AnonymisedAt, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Timeline{}, pgx.ErrNoRows
		}

		return Timeline{}, fmt.Errorf("visatimeline: get timeline: %w", err)
	}

	return t, nil
}

// ListUserTimelines returns all non-deleted timelines owned by a user within
// a tenant, newest first.
func (s *Store) ListUserTimelines(ctx context.Context, userID, tenantID pgtype.UUID) ([]Timeline, error) {
	// Join milestones via COALESCE(LATERAL json_agg, '[]') so the API can
	// return timelines with their milestones in a single round-trip (§83
	// defines no separate list-milestones route). Milestones are ordered
	// chronologically inside the LATERAL subquery.
	const q = `
SELECT t.id, t.user_id, t.origin_space_id, t.destination_space_id,
       t.visa_category, t.consulate, t.application_method, t.outcome, t.is_shared,
       t.anonymised_at, t.created_at, t.updated_at, t.deleted_at,
       COALESCE(ms.milestones, '[]'::json) AS milestones
FROM visa_timelines t
LEFT JOIN LATERAL (
    SELECT json_agg(json_build_object(
        'id', m.id,
        'timeline_id', m.timeline_id,
        'milestone_type', m.milestone_type,
        'occurred_at', m.occurred_at,
        'notes', m.notes,
        'created_at', m.created_at
    ) ORDER BY m.occurred_at ASC) AS milestones
    FROM visa_timeline_milestones m
    WHERE m.timeline_id = t.id
) ms ON TRUE
WHERE t.user_id = $1 AND t.deleted_at IS NULL
ORDER BY t.created_at DESC
`

	rows, err := s.pool.Reader.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: list user timelines: %w", err)
	}

	defer rows.Close()

	var out []Timeline

	for rows.Next() {
		var t Timeline

		var milestonesJSON []byte
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.OriginSpaceID, &t.DestinationSpaceID,
			&t.VisaCategory, &t.Consulate, &t.ApplicationMethod, &t.Outcome, &t.IsShared,
			&t.AnonymisedAt, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
			&milestonesJSON,
		); err != nil {
			return nil, fmt.Errorf("visatimeline: scan timeline: %w", err)
		}

		t.Milestones = decodeMilestonesJSON(milestonesJSON)
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
		// §83/ADR-091: stamp anonymised_at when the timeline is shared
		// (anonymisation audit trail). Only set when sharing is enabled.
		if *input.IsShared {
			sets = append(sets, "anonymised_at = now()")
		}
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
	args = append(args, id)

	q := fmt.Sprintf(`
UPDATE visa_timelines SET %s WHERE id = $%d AND deleted_at IS NULL
`, strings.Join(sets, ", "), argIdx)

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
WHERE id = $1 AND deleted_at IS NULL
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
INSERT INTO visa_timeline_milestones (timeline_id, milestone_type, occurred_at, notes)
VALUES ($1, $2, $3, $4)
RETURNING id, timeline_id, milestone_type, occurred_at, notes, created_at
`

	var m Milestone

	err := s.pool.Writer.QueryRow(
		ctx, q,
		timelineID, milestoneType, occurredAt, notes,
	).Scan(
		&m.ID, &m.TimelineID, &m.MilestoneType, &m.OccurredAt, &m.Notes, &m.CreatedAt,
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

	args = append(args, milestoneID)

	q := fmt.Sprintf(`
UPDATE visa_timeline_milestones SET %s WHERE id = $%d
`, strings.Join(sets, ", "), argIdx)

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
SELECT id, timeline_id, milestone_type, occurred_at, notes, created_at
FROM visa_timeline_milestones
WHERE timeline_id = $1
ORDER BY occurred_at ASC, created_at ASC
`

	rows, err := s.pool.Reader.Query(ctx, q, timelineID)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: list milestones: %w", err)
	}

	defer rows.Close()

	var out []Milestone

	for rows.Next() {
		var m Milestone
		if err := rows.Scan(
			&m.ID, &m.TimelineID, &m.MilestoneType, &m.OccurredAt, &m.Notes, &m.CreatedAt,
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
// stats are read from the pre-aggregated visa_processing_stats table. When
// months > 0, only periods within the last `months` months are returned.
func (s *Store) GetStats(ctx context.Context, tenantID, originSpaceID, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]ProcessingStat, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

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

	if consulate != "" {
		conditions = append(conditions, fmt.Sprintf("consulate = $%d", argIdx))
		args = append(args, consulate)
		argIdx++
	}

	if months > 0 {
		conditions = append(conditions, fmt.Sprintf("period_start >= now() - interval '%d months'", months))
	}

	whereClause := strings.Join(conditions, " AND ")

	q := fmt.Sprintf(`
SELECT id, origin_space_id, destination_space_id, visa_category,
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
			&st.ID, &st.OriginSpaceID, &st.DestinationSpaceID,
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
// destination space + visa category over the last `months` months. When
// consulate is non-empty, the trend is filtered to that consulate only.
func (s *Store) GetTrend(ctx context.Context, tenantID, destSpaceID pgtype.UUID, visaCategory, consulate string, months int) ([]TrendPoint, error) {
	if months <= 0 {
		months = 12
	}

	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

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

	if consulate != "" {
		conditions = append(conditions, fmt.Sprintf("consulate = $%d", argIdx))
		args = append(args, consulate)
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
	TenantID           pgtype.UUID
	UserID             pgtype.UUID // optional: filter by submitter
	DestinationSpaceID pgtype.UUID // optional
	VisaCategory       string      // optional
	Outcome            string      // optional
	Q                  string      // optional: search visa_category/consulate
	Sort               string      // optional: sort column (created_at|updated_at|visa_category|outcome)
	Order              string      // optional: asc|desc (default desc)
	Limit              int32
	Offset             int32
}

// adminListSortColumns is the whitelist of columns the admin list may sort
// by (geo-aware tool pattern: "all admin tables must have filter + sort +
// paginate"). The column key maps to the SQL expression used in ORDER BY.
var adminListSortColumns = map[string]string{
	"created_at":    "t.created_at",
	"updated_at":    "t.updated_at",
	"visa_category": "t.visa_category",
	"outcome":       "t.outcome",
}

// AdminList returns all timelines (any share state) for the moderation queue.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]Timeline, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

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

	// Resolve the sort column from the whitelist (geo-aware tool pattern:
	// "all admin tables must have filter + sort + paginate"). Default is
	// created_at DESC. The order direction is clamped to asc/desc.
	sortExpr := "t.created_at"
	if col, ok := adminListSortColumns[f.Sort]; ok {
		sortExpr = col
	}

	orderDir := "DESC"
	if strings.EqualFold(f.Order, "asc") {
		orderDir = "ASC"
	}

	listQ := fmt.Sprintf(`
SELECT t.id, t.user_id, t.origin_space_id, t.destination_space_id,
       t.visa_category, t.consulate, t.application_method, t.outcome, t.is_shared,
       t.anonymised_at, t.created_at, t.updated_at, t.deleted_at,
       COALESCE(ms.milestones, '[]'::json) AS milestones
FROM visa_timelines t
LEFT JOIN LATERAL (
    SELECT json_agg(json_build_object(
        'id', m.id,
        'timeline_id', m.timeline_id,
        'milestone_type', m.milestone_type,
        'occurred_at', m.occurred_at,
        'notes', m.notes,
        'created_at', m.created_at
    ) ORDER BY m.occurred_at ASC) AS milestones
    FROM visa_timeline_milestones m
    WHERE m.timeline_id = t.id
) ms ON TRUE
WHERE %s
ORDER BY %s %s
LIMIT $%d OFFSET $%d
`, whereClause, sortExpr, orderDir, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("visatimeline: admin list: %w", err)
	}
	defer rows.Close()

	var out []Timeline

	for rows.Next() {
		var t Timeline

		var milestonesJSON []byte
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.OriginSpaceID, &t.DestinationSpaceID,
			&t.VisaCategory, &t.Consulate, &t.ApplicationMethod, &t.Outcome, &t.IsShared,
			&t.AnonymisedAt, &t.CreatedAt, &t.UpdatedAt, &t.DeletedAt,
			&milestonesJSON,
		); err != nil {
			return nil, 0, fmt.Errorf("visatimeline: admin scan timeline: %w", err)
		}

		t.Milestones = decodeMilestonesJSON(milestonesJSON)
		out = append(out, t)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("visatimeline: admin iterate timelines: %w", err)
	}

	countQ := "SELECT COUNT(*) FROM visa_timelines WHERE " + whereClause
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("visatimeline: admin count timelines: %w", err)
	}

	return out, total, nil
}

// RefreshStats recomputes the visa_processing_stats aggregate table from
// shared, non-deleted visa_timelines and their milestones. For each
// (tenant_id, origin_space_id, destination_space_id, visa_category,
// consulate, month) bucket, computes sample_count, avg_days, median_days,
// p10_days, p90_days of the processing span (decision or visa_issued
// milestone occurred_at minus the earliest applied/submitted milestone
// occurred_at). Timelines without a decision/visa_issued milestone are
// excluded (still in progress). Called nightly by the
// visa_timeline:refresh_stats Asynq task (§83, ADR-091).
//
// The aggregate is computed fresh each run: existing rows for the affected
// period are deleted then re-inserted in a single transaction.
func (s *Store) RefreshStats(ctx context.Context) (int64, error) {
	// Compute processing spans per timeline. A span is the number of days
	// between the earliest "applied" milestone and the latest
	// "decision"/"visa_issued" milestone. Timelines sharing the same
	// (tenant, origin, dest, visa_category, consulate) are bucketed by
	// the month of the decision date.
	const q = `
WITH spans AS (
    SELECT
        t.origin_space_id,
        t.destination_space_id,
        t.visa_category,
        t.consulate,
        date_trunc('month', dec_m.occurred_at)::date AS period_start,
        EXTRACT(EPOCH FROM (dec_m.occurred_at - first_m.first_occurred))::numeric / 86400 AS days_span
    FROM visa_timelines t
    JOIN LATERAL (
        SELECT occurred_at
        FROM visa_timeline_milestones m
        WHERE m.timeline_id = t.id
          AND m.milestone_type IN ('applied')
        ORDER BY occurred_at ASC
        LIMIT 1
    ) first_m ON TRUE
    JOIN LATERAL (
        SELECT occurred_at
        FROM visa_timeline_milestones m
        WHERE m.timeline_id = t.id
          AND m.milestone_type IN ('decision', 'visa_issued')
        ORDER BY occurred_at DESC
        LIMIT 1
    ) dec_m ON TRUE
    WHERE t.is_shared = TRUE
      AND t.deleted_at IS NULL
      AND t.outcome IN ('approved', 'refused', 'withdrawn')
),
buckets AS (
    SELECT
        origin_space_id,
        destination_space_id,
        visa_category,
        consulate,
        period_start,
        COUNT(*) AS sample_count,
        AVG(days_span)::numeric(6,1) AS avg_days,
        PERCENTILE_CONT(0.5)  WITHIN GROUP (ORDER BY days_span)::numeric(6,1) AS median_days,
        PERCENTILE_CONT(0.10) WITHIN GROUP (ORDER BY days_span)::numeric(6,1) AS p10_days,
        PERCENTILE_CONT(0.90) WITHIN GROUP (ORDER BY days_span)::numeric(6,1) AS p90_days
    FROM spans
GROUP BY  origin_space_id, destination_space_id, visa_category, consulate, period_start
)
INSERT INTO visa_processing_stats (
    origin_space_id, destination_space_id, visa_category,
    consulate, period_start, sample_count, avg_days, median_days,
    p10_days, p90_days, refreshed_at
)
SELECT
    origin_space_id, destination_space_id, visa_category,
    consulate, period_start, sample_count, avg_days, median_days,
    p10_days, p90_days, now()
FROM buckets
ON CONFLICT (origin_space_id, destination_space_id, visa_category, consulate, period_start)
DO UPDATE SET
    sample_count = EXCLUDED.sample_count,
    avg_days = EXCLUDED.avg_days,
    median_days = EXCLUDED.median_days,
    p10_days = EXCLUDED.p10_days,
    p90_days = EXCLUDED.p90_days,
    refreshed_at = EXCLUDED.refreshed_at
`

	cmd, err := s.pool.Writer.Exec(ctx, q)
	if err != nil {
		return 0, fmt.Errorf("visatimeline: refresh stats: %w", err)
	}

	return cmd.RowsAffected(), nil
}

// InProgressTimelineOwners — InProgressTimelineOwner identifies (user_id, tenant_id) pairs whose shared
// timeline has no decision/visa_issued milestone yet, for the weekly
// visa_timeline:prompt_update nudge (§83, ADR-091). Only timelines updated
// more than 7 days ago are returned so the prompt is not spammy.
func (s *Store) InProgressTimelineOwners(ctx context.Context) ([]struct {
	UserID   pgtype.UUID
	TenantID pgtype.UUID
}, error,
) {
	const q = `
SELECT DISTINCT t.user_id
FROM visa_timelines t
WHERE t.deleted_at IS NULL
  AND t.user_id IS NOT NULL
  AND t.outcome = 'pending'
  AND t.updated_at < now() - interval '7 days'
  AND NOT EXISTS (
      SELECT 1 FROM visa_timeline_milestones m
      WHERE m.timeline_id = t.id
        AND m.milestone_type IN ('decision', 'visa_issued')
  )
`

	rows, err := s.pool.Reader.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("visatimeline: in-progress timeline owners: %w", err)
	}

	defer rows.Close()

	var out []struct {
		UserID   pgtype.UUID
		TenantID pgtype.UUID
	}

	for rows.Next() {
		var r struct {
			UserID   pgtype.UUID
			TenantID pgtype.UUID
		}

		if err := rows.Scan(&r.UserID); err != nil {
			return nil, fmt.Errorf("visatimeline: scan in-progress owner: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("visatimeline: iterate in-progress owners: %w", err)
	}

	return out, nil
}

// UserAccountAge returns the user's created_at timestamp for the anti-gaming
// account-age gate (§83: account age ≥ visa_timeline.min_account_age_days).
// Returns pgtype.Timestamptz{} (Valid=false) if the user does not exist.
func (s *Store) UserAccountAge(ctx context.Context, tenantID, userID pgtype.UUID) (pgtype.Timestamptz, error) {
	const q = `SELECT created_at FROM users WHERE id = $1`

	var ts pgtype.Timestamptz
	if err := s.pool.Reader.QueryRow(ctx, q, userID).Scan(&ts); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.Timestamptz{}, nil
		}

		return pgtype.Timestamptz{}, fmt.Errorf("visatimeline: user account age: %w", err)
	}

	return ts, nil
}

// FlagImplausibleSequence records an audit_logs entry when a member
// attempts to add a milestone that violates the canonical chronological
// order (§83 Privacy & Anti-Gaming: "Implausible milestone sequences flagged
// automatically and held for moderator review"). The milestone is rejected
// (not stored), and this audit row lets moderators review flagged attempts.
// Ref: japaconnect-engineering/sections/83-visa-timeline-tracker.md.
func (s *Store) FlagImplausibleSequence(ctx context.Context, tenantID, userID, timelineID pgtype.UUID, milestoneType string, occurredAt pgtype.Timestamptz) error {
	metadata := fmt.Sprintf(`{"timeline_id":"%s","milestone_type":%q,"occurred_at":"%s"}`,
		timelineID.String(), milestoneType, occurredAt.Time.UTC().Format(time.RFC3339))
	targetType := "visa_timeline_milestone"

	if _, err := s.pool.Writer.Exec(ctx, `
INSERT INTO audit_logs (actor_user_id, action_type, target_type, target_id, metadata) VALUES ($1, $2, $3, $4, $5::jsonb)
`, userID, "visa_timeline.implausible_sequence", targetType, timelineID, metadata); err != nil {
		return fmt.Errorf("visatimeline: flag implausible sequence: %w", err)
	}

	return nil
}

// decodeMilestonesJSON parses the JSON array produced by the LATERAL
// json_agg in ListUserTimelines / AdminList into []Milestone. Returns an
// empty slice (not nil) when the JSON is "[]" or null so callers always
// get a non-nil slice.
func decodeMilestonesJSON(raw []byte) []Milestone {
	if len(raw) == 0 || string(raw) == "null" {
		return []Milestone{}
	}

	var raws []struct {
		ID            string  `json:"id"`
		TenantID      string  `json:"tenant_id"`
		TimelineID    string  `json:"timeline_id"`
		MilestoneType string  `json:"milestone_type"`
		OccurredAt    string  `json:"occurred_at"`
		Notes         *string `json:"notes"`
		CreatedAt     string  `json:"created_at"`
	}

	if err := json.Unmarshal(raw, &raws); err != nil {
		return []Milestone{}
	}

	out := make([]Milestone, 0, len(raws))

	for _, r := range raws {
		var m Milestone
		_ = m.ID.Scan(r.ID)
		_ = m.TimelineID.Scan(r.TimelineID)

		m.MilestoneType = r.MilestoneType
		if t, err := time.Parse(time.RFC3339Nano, r.OccurredAt); err == nil {
			m.OccurredAt = pgtype.Timestamptz{Time: t, Valid: true}
		}

		m.Notes = r.Notes
		if t, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err == nil {
			m.CreatedAt = pgtype.Timestamptz{Time: t, Valid: true}
		}

		out = append(out, m)
	}

	return out
}

// CreateProcessingTimeInput holds the fields for inserting a processing-time
// stat row. It is used by the service layer after the request has been parsed.
type CreateProcessingTimeInput struct {
	TenantID           pgtype.UUID
	OriginSpaceID      pgtype.UUID
	DestinationSpaceID pgtype.UUID
	VisaCategory       string
	Consulate          *string
	PeriodStart        pgtype.Timestamptz
	SampleCount        int32
	AvgDays            *float64
	MedianDays         *float64
	P10Days            *float64
	P90Days            *float64
}

// CreateProcessingTime inserts a new visa_processing_stats row.
func (s *Store) CreateProcessingTime(ctx context.Context, in CreateProcessingTimeInput) (ProcessingStat, error) {
	const q = `
INSERT INTO visa_processing_stats (
    origin_space_id, destination_space_id, visa_category,
    consulate, period_start, sample_count, avg_days, median_days,
    p10_days, p90_days, refreshed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
RETURNING id, origin_space_id, destination_space_id, visa_category,
    consulate, period_start, sample_count, avg_days, median_days,
    p10_days, p90_days, refreshed_at
`

	var r ProcessingStat

	err := s.pool.Writer.QueryRow(
		ctx, q,
		in.OriginSpaceID, in.DestinationSpaceID, in.VisaCategory,
		in.Consulate, in.PeriodStart, in.SampleCount, in.AvgDays, in.MedianDays,
		in.P10Days, in.P90Days,
	).Scan(
		&r.ID, &r.OriginSpaceID, &r.DestinationSpaceID,
		&r.VisaCategory, &r.Consulate, &r.PeriodStart, &r.SampleCount,
		&r.AvgDays, &r.MedianDays, &r.P10Days, &r.P90Days, &r.RefreshedAt,
	)
	if err != nil {
		return ProcessingStat{}, fmt.Errorf("visatimeline: create processing time: %w", err)
	}

	return r, nil
}

// UpdateProcessingTimeInput holds the mutable fields for a processing-time
// stat update. Only non-nil pointers are applied.
type UpdateProcessingTimeInput struct {
	OriginSpaceID      *pgtype.UUID
	DestinationSpaceID *pgtype.UUID
	VisaCategory       *string
	Consulate          *string
	PeriodStart        *pgtype.Timestamptz
	SampleCount        *int32
	AvgDays            *float64
	MedianDays         *float64
	P10Days            *float64
	P90Days            *float64
}

// UpdateProcessingTime updates a visa_processing_stats row within a tenant.
func (s *Store) UpdateProcessingTime(ctx context.Context, id, tenantID pgtype.UUID, in UpdateProcessingTimeInput) (ProcessingStat, error) {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if in.OriginSpaceID != nil {
		sets = append(sets, fmt.Sprintf("origin_space_id = $%d", argIdx))
		args = append(args, *in.OriginSpaceID)
		argIdx++
	}

	if in.DestinationSpaceID != nil {
		sets = append(sets, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, *in.DestinationSpaceID)
		argIdx++
	}

	if in.VisaCategory != nil {
		sets = append(sets, fmt.Sprintf("visa_category = $%d", argIdx))
		args = append(args, *in.VisaCategory)
		argIdx++
	}

	if in.Consulate != nil {
		sets = append(sets, fmt.Sprintf("consulate = $%d", argIdx))
		args = append(args, *in.Consulate)
		argIdx++
	}

	if in.PeriodStart != nil {
		sets = append(sets, fmt.Sprintf("period_start = $%d", argIdx))
		args = append(args, *in.PeriodStart)
		argIdx++
	}

	if in.SampleCount != nil {
		sets = append(sets, fmt.Sprintf("sample_count = $%d", argIdx))
		args = append(args, *in.SampleCount)
		argIdx++
	}

	if in.AvgDays != nil {
		sets = append(sets, fmt.Sprintf("avg_days = $%d", argIdx))
		args = append(args, *in.AvgDays)
		argIdx++
	}

	if in.MedianDays != nil {
		sets = append(sets, fmt.Sprintf("median_days = $%d", argIdx))
		args = append(args, *in.MedianDays)
		argIdx++
	}

	if in.P10Days != nil {
		sets = append(sets, fmt.Sprintf("p10_days = $%d", argIdx))
		args = append(args, *in.P10Days)
		argIdx++
	}

	if in.P90Days != nil {
		sets = append(sets, fmt.Sprintf("p90_days = $%d", argIdx))
		args = append(args, *in.P90Days)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetStatByID(ctx, id, tenantID)
	}

	sets = append(sets, "refreshed_at = now()")
	args = append(args, id)

	q := fmt.Sprintf(
		"UPDATE visa_processing_stats SET %s WHERE id = $%d RETURNING id, origin_space_id, destination_space_id, visa_category, consulate, period_start, sample_count, avg_days, median_days, p10_days, p90_days, refreshed_at",
		strings.Join(sets, ", "),
		argIdx,
	)

	var r ProcessingStat

	err := s.pool.Writer.QueryRow(ctx, q, args...).Scan(
		&r.ID, &r.OriginSpaceID, &r.DestinationSpaceID,
		&r.VisaCategory, &r.Consulate, &r.PeriodStart, &r.SampleCount,
		&r.AvgDays, &r.MedianDays, &r.P10Days, &r.P90Days, &r.RefreshedAt,
	)
	if err != nil {
		return ProcessingStat{}, fmt.Errorf("visatimeline: update processing time: %w", err)
	}

	return r, nil
}

// GetStatByID returns a single visa_processing_stats row by id and tenant.
func (s *Store) GetStatByID(ctx context.Context, id, tenantID pgtype.UUID) (ProcessingStat, error) {
	const q = `
SELECT id, origin_space_id, destination_space_id, visa_category,
       consulate, period_start, sample_count, avg_days, median_days,
       p10_days, p90_days, refreshed_at
FROM visa_processing_stats
WHERE id = $1
`

	var r ProcessingStat

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.OriginSpaceID, &r.DestinationSpaceID,
		&r.VisaCategory, &r.Consulate, &r.PeriodStart, &r.SampleCount,
		&r.AvgDays, &r.MedianDays, &r.P10Days, &r.P90Days, &r.RefreshedAt,
	)
	if err != nil {
		return ProcessingStat{}, fmt.Errorf("visatimeline: get stat by id: %w", err)
	}

	return r, nil
}

// DeleteProcessingTime deletes a visa_processing_stats row within a tenant.
func (s *Store) DeleteProcessingTime(ctx context.Context, id, tenantID pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `DELETE FROM visa_processing_stats WHERE id = $1`, id, )
	if err != nil {
		return fmt.Errorf("visatimeline: delete processing time: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
