// Package taxcalculator implements the Tax & Financial Obligations
// Calculator (§100, ADR-108).
// // The package exposes two domain tables. tax_treaty_pairs records whether
// a double-taxation treaty exists between an origin and destination space
// (country-level spaces per ADR-053). tax_obligation_guides stores
// per-employment-status tax obligation guides — including residency
// triggers, first-year obligations, an action checklist (JSONB), and
// Human-in-the-Loop review tracking for AI-generated drafts. The package
// provides public lookup (by destination/origin/employment), public
// treaty-status lookup, admin CRUD, a Content Agent draft placeholder,
// and HITL approval.
// // Ref: japaconnect-engineering/sections/100-tax-calculator.md
// //	japaconnect-engineering/adr/ADR-108-tax-calculator.md
package taxcalculator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Employment statuses (§100).
const (
	EmploymentEmployed     = "employed"
	EmploymentSelfEmployed = "self_employed"
	EmploymentContractor   = "contractor"
	EmploymentUnemployed   = "unemployed"
)

// Guide statuses (§100).
const (
	GuideStatusDraft     = "draft"
	GuideStatusAIDraft   = "ai_draft"
	GuideStatusPending   = "pending"
	GuideStatusPublished = "published"
	GuideStatusRejected  = "rejected"
)

// Store is the data-access layer for tax_treaty_pairs and
// tax_obligation_guides. It uses direct pgx queries because these are new
// tables without sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a tax calculator store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// GuideRow is the row shape returned by list and get-by-id queries for
// tax_obligation_guides, joined with spaces for origin/destination names.
type GuideRow struct {
	ID pgtype.UUID

	OriginSpaceID        pgtype.UUID
	OriginSpaceName      *string
	DestinationSpaceID   pgtype.UUID
	DestinationSpaceName *string
	EmploymentStatus     string
	ResidencyTrigger     *string
	WorldwideIncomeTaxed *bool
	FirstYearObligations *string
	ActionChecklist      []byte // JSONB array
	Status               string
	IsActive             bool
	AIGenerated          bool
	AIModel              *string
	AIPromptHash         *string
	HITLReviewedBy       pgtype.UUID
	HITLReviewedAt       pgtype.Timestamptz
	HITLReviewNotes      *string
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
}

// TreatyRow is the row shape returned by treaty lookups for
// tax_treaty_pairs, joined with spaces for origin/destination names.
type TreatyRow struct {
	ID pgtype.UUID

	OriginSpaceID        pgtype.UUID
	OriginSpaceName      *string
	DestinationSpaceID   pgtype.UUID
	DestinationSpaceName *string
	TreatyExists         bool
	TreatySummary        *string
	TreatySourceURL      *string
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
}

// ListGuidesFilter holds the filter parameters for listing published guides.
type ListGuidesFilter struct {
	DestinationSpaceID pgtype.UUID // optional
	OriginSpaceID      pgtype.UUID // optional
	EmploymentStatus   string      // optional
	OnlyActive         bool        // default true for public list
	Limit              int32
	Offset             int32
}

// ListGuides returns a paginated, filtered list of tax obligation guides.
// Public callers pass OnlyActive=true and rely on status='published'.
func (s *Store) ListGuides(ctx context.Context, f ListGuidesFilter) ([]GuideRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("g.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.OriginSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("g.origin_space_id = $%d", argIdx))
		args = append(args, f.OriginSpaceID)
		argIdx++
	}

	if f.EmploymentStatus != "" {
		conditions = append(conditions, fmt.Sprintf("g.employment_status = $%d", argIdx))
		args = append(args, f.EmploymentStatus)
		argIdx++
	}

	if f.OnlyActive {
		conditions = append(conditions, "g.is_active = true")
		conditions = append(conditions, "g.status = 'published'")
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

	listQuery := fmt.Sprintf(`
SELECT g.id, g.origin_space_id, os.name AS origin_space_name,
       g.destination_space_id, ds.name AS destination_space_name,
       g.employment_status, g.residency_trigger, g.worldwide_income_taxed,
       g.first_year_obligations, g.action_checklist,
       g.status, g.is_active, g.ai_generated, g.ai_model, g.ai_prompt_hash,
       g.hitl_reviewed_by, g.hitl_reviewed_at, g.hitl_review_notes,
       g.created_at, g.updated_at
FROM tax_obligation_guides g
LEFT JOIN spaces os ON os.id = g.origin_space_id
LEFT JOIN spaces ds ON ds.id = g.destination_space_id
WHERE %s
ORDER BY g.updated_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("taxcalculator: list guides: %w", err)
	}
	defer rows.Close()

	var out []GuideRow

	for rows.Next() {
		var r GuideRow
		if err := rows.Scan(
			&r.ID, &r.OriginSpaceID, &r.OriginSpaceName,
			&r.DestinationSpaceID, &r.DestinationSpaceName,
			&r.EmploymentStatus, &r.ResidencyTrigger, &r.WorldwideIncomeTaxed,
			&r.FirstYearObligations, &r.ActionChecklist,
			&r.Status, &r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("taxcalculator: scan guide row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("taxcalculator: iterate guide rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM tax_obligation_guides g
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("taxcalculator: count guides: %w", err)
	}

	return out, total, nil
}

// GetGuideByID returns a single tax obligation guide by its primary key.
func (s *Store) GetGuideByID(ctx context.Context, id pgtype.UUID) (GuideRow, error) {
	const q = `
SELECT g.id, g.origin_space_id, os.name AS origin_space_name,
       g.destination_space_id, ds.name AS destination_space_name,
       g.employment_status, g.residency_trigger, g.worldwide_income_taxed,
       g.first_year_obligations, g.action_checklist,
       g.status, g.is_active, g.ai_generated, g.ai_model, g.ai_prompt_hash,
       g.hitl_reviewed_by, g.hitl_reviewed_at, g.hitl_review_notes,
       g.created_at, g.updated_at
FROM tax_obligation_guides g
LEFT JOIN spaces os ON os.id = g.origin_space_id
LEFT JOIN spaces ds ON ds.id = g.destination_space_id
WHERE g.id = $1
LIMIT 1
`

	var r GuideRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.OriginSpaceID, &r.OriginSpaceName,
		&r.DestinationSpaceID, &r.DestinationSpaceName,
		&r.EmploymentStatus, &r.ResidencyTrigger, &r.WorldwideIncomeTaxed,
		&r.FirstYearObligations, &r.ActionChecklist,
		&r.Status, &r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return GuideRow{}, pgx.ErrNoRows
		}

		return GuideRow{}, fmt.Errorf("taxcalculator: get guide by id: %w", err)
	}

	return r, nil
}

// GetTreatyByPair returns the tax_treaty_pairs row for a given
// origin/destination space pair within a tenant.
func (s *Store) GetTreatyByPair(ctx context.Context, originSpaceID, destSpaceID pgtype.UUID) (TreatyRow, error) {
	const q = `
SELECT t.id, t.origin_space_id, os.name AS origin_space_name,
       t.destination_space_id, ds.name AS destination_space_name,
       t.treaty_exists, t.treaty_summary, t.treaty_source_url,
       t.created_at, t.updated_at
FROM tax_treaty_pairs t
LEFT JOIN spaces os ON os.id = t.origin_space_id
LEFT JOIN spaces ds ON ds.id = t.destination_space_id
WHERE t.origin_space_id = $1 AND t.destination_space_id = $2
LIMIT 1
`

	var r TreatyRow

	err := s.pool.Reader.QueryRow(ctx, q, originSpaceID, destSpaceID).Scan(
		&r.ID, &r.OriginSpaceID, &r.OriginSpaceName,
		&r.DestinationSpaceID, &r.DestinationSpaceName,
		&r.TreatyExists, &r.TreatySummary, &r.TreatySourceURL,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreatyRow{}, pgx.ErrNoRows
		}

		return TreatyRow{}, fmt.Errorf("taxcalculator: get treaty by pair: %w", err)
	}

	return r, nil
}

// CreateGuideInput holds the fields needed to insert a new
// tax_obligation_guides row.
type CreateGuideInput struct {
	OriginSpaceID        pgtype.UUID
	DestinationSpaceID   pgtype.UUID
	EmploymentStatus     string
	ResidencyTrigger     *string
	WorldwideIncomeTaxed *bool
	FirstYearObligations *string
	ActionChecklist      []byte // JSONB
	Status               string
	IsActive             bool
	AIGenerated          bool
	AIModel              *string
	AIPromptHash         *string
}

// CreateGuide inserts a new tax_obligation_guides row and returns the
// newly created row.
func (s *Store) CreateGuide(ctx context.Context, in CreateGuideInput) (GuideRow, error) {
	checklist := in.ActionChecklist
	if checklist == nil {
		checklist = []byte("[]")
	}
	// BUG-004: HITL is mandatory for all tax content (§100/ADR-108).
	// New guides default to 'draft' — only ApproveGuide may set
	// 'published'. Reject any caller-supplied 'published' status here.
	status := in.Status
	if status == "" || status == GuideStatusPublished {
		status = GuideStatusDraft
	}

	const q = `
INSERT INTO tax_obligation_guides (
    origin_space_id, destination_space_id, employment_status,
    residency_trigger, worldwide_income_taxed, first_year_obligations,
    action_checklist, status, is_active, ai_generated, ai_model, ai_prompt_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id
`

	var id pgtype.UUID
	if err := s.pool.Writer.QueryRow(
		ctx, q, (pgtype.UUID{}),

		in.OriginSpaceID, in.DestinationSpaceID, in.EmploymentStatus,
		in.ResidencyTrigger, in.WorldwideIncomeTaxed, in.FirstYearObligations,
		checklist, status, in.IsActive, in.AIGenerated, in.AIModel, in.AIPromptHash,
	).Scan(&id); err != nil {
		return GuideRow{}, fmt.Errorf("taxcalculator: create guide: %w", err)
	}

	row, err := s.GetGuideByID(ctx, id)
	if err != nil {
		return GuideRow{}, fmt.Errorf("taxcalculator: create guide: reload: %w", err)
	}

	return row, nil
}

// UpdateGuideInput holds the mutable fields for a guide update. Only
// non-nil fields are applied.
type UpdateGuideInput struct {
	ResidencyTrigger     *string
	WorldwideIncomeTaxed *bool
	FirstYearObligations *string
	ActionChecklist      []byte // JSONB; nil means leave unchanged
	Status               *string
	IsActive             *bool
}

// UpdateGuide updates the mutable fields on a tax_obligation_guides row.
func (s *Store) UpdateGuide(ctx context.Context, id pgtype.UUID, in UpdateGuideInput) error {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if in.ResidencyTrigger != nil {
		sets = append(sets, fmt.Sprintf("residency_trigger = $%d", argIdx))
		args = append(args, *in.ResidencyTrigger)
		argIdx++
	}

	if in.WorldwideIncomeTaxed != nil {
		sets = append(sets, fmt.Sprintf("worldwide_income_taxed = $%d", argIdx))
		args = append(args, *in.WorldwideIncomeTaxed)
		argIdx++
	}

	if in.FirstYearObligations != nil {
		sets = append(sets, fmt.Sprintf("first_year_obligations = $%d", argIdx))
		args = append(args, *in.FirstYearObligations)
		argIdx++
	}

	if in.ActionChecklist != nil {
		sets = append(sets, fmt.Sprintf("action_checklist = $%d", argIdx))
		args = append(args, in.ActionChecklist)
		argIdx++
	}

	if in.Status != nil {
		// BUG-004: Only ApproveGuide may set status='published'.
		// UpdateGuide is for editing content fields, not for
		// publishing. Silently downgrade any 'published' to 'pending'
		// so the HITL gate is never bypassed via the update path.
		st := *in.Status
		if st == GuideStatusPublished {
			st = GuideStatusPending
		}

		sets = append(sets, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, st)
		argIdx++
	}

	if in.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *in.IsActive)
		argIdx++
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = now()")

	q := fmt.Sprintf(`
UPDATE tax_obligation_guides
SET %s
WHERE id = $%d
`, strings.Join(sets, ", "), argIdx)

	args = append(args, id)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("taxcalculator: update guide: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// DeleteGuide deletes a tax_obligation_guides row by id and .
func (s *Store) DeleteGuide(ctx context.Context, id pgtype.UUID) error {
	const q = `
DELETE FROM tax_obligation_guides
WHERE id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("taxcalculator: delete guide: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ApproveGuide performs the Human-in-the-Loop approval: sets
// hitl_reviewed_by, hitl_reviewed_at = now(), status = 'published', and
// optional review notes.
func (s *Store) ApproveGuide(ctx context.Context, id, reviewerID pgtype.UUID, notes *string) error {
	const q = `
UPDATE tax_obligation_guides
SET hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    hitl_review_notes = $3,
    status = 'published',
    updated_at = now()
WHERE id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, reviewerID, notes)
	if err != nil {
		return fmt.Errorf("taxcalculator: approve guide: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// TreatyUpsertInput holds the fields for creating or updating a
// tax_treaty_pairs row via upsert.
type TreatyUpsertInput struct {
	OriginSpaceID      pgtype.UUID
	DestinationSpaceID pgtype.UUID
	TreatyExists       bool
	TreatySummary      *string
	TreatySourceURL    *string
}

// CreateOrUpdateTreaty upserts a tax_treaty_pairs row keyed by
// ( origin_space_id, destination_space_id). On conflict it
// updates the mutable fields and returns the resulting row.
func (s *Store) CreateOrUpdateTreaty(ctx context.Context, in TreatyUpsertInput) (TreatyRow, error) {
	const q = `
INSERT INTO tax_treaty_pairs (
    origin_space_id, destination_space_id,
    treaty_exists, treaty_summary, treaty_source_url
) VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (origin_space_id, destination_space_id)
DO UPDATE SET
    treaty_exists     = EXCLUDED.treaty_exists,
    treaty_summary    = EXCLUDED.treaty_summary,
    treaty_source_url = EXCLUDED.treaty_source_url,
    updated_at        = now()
RETURNING id
`

	var id pgtype.UUID
	if err := s.pool.Writer.QueryRow(
		ctx, q, (pgtype.UUID{}),

		in.OriginSpaceID, in.DestinationSpaceID,
		in.TreatyExists, in.TreatySummary, in.TreatySourceURL,
	).Scan(&id); err != nil {
		return TreatyRow{}, fmt.Errorf("taxcalculator: upsert treaty: %w", err)
	}

	row, err := s.GetTreatyByPair(ctx, in.OriginSpaceID, in.DestinationSpaceID)
	if err != nil {
		return TreatyRow{}, fmt.Errorf("taxcalculator: upsert treaty: reload: %w", err)
	}

	return row, nil
}

// UpdateTreatyInput holds the mutable fields for a tax_treaty_pairs
// update. Unlike TreatyUpsertInput, the row is identified by id.
type UpdateTreatyInput struct {
	TreatyExists    *bool
	TreatySummary   *string
	TreatySourceURL *string
}

// getTreatyByID returns a tax_treaty_pairs row by its primary key and
// , joined with spaces for origin/destination names.
func (s *Store) getTreatyByID(ctx context.Context, id pgtype.UUID) (TreatyRow, error) {
	const q = `
SELECT t.id, t.origin_space_id, os.name AS origin_space_name,
       t.destination_space_id, ds.name AS destination_space_name,
       t.treaty_exists, t.treaty_summary, t.treaty_source_url,
       t.created_at, t.updated_at
FROM tax_treaty_pairs t
LEFT JOIN spaces os ON os.id = t.origin_space_id
LEFT JOIN spaces ds ON ds.id = t.destination_space_id
WHERE t.id = $1
LIMIT 1
`

	var r TreatyRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.OriginSpaceID, &r.OriginSpaceName,
		&r.DestinationSpaceID, &r.DestinationSpaceName,
		&r.TreatyExists, &r.TreatySummary, &r.TreatySourceURL,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreatyRow{}, pgx.ErrNoRows
		}

		return TreatyRow{}, fmt.Errorf("taxcalculator: get treaty by id: %w", err)
	}

	return r, nil
}

// UpdateTreaty updates the mutable fields on a tax_treaty_pairs row
// identified by id and , returning the updated row.
func (s *Store) UpdateTreaty(ctx context.Context, id pgtype.UUID, in UpdateTreatyInput) (TreatyRow, error) {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if in.TreatyExists != nil {
		sets = append(sets, fmt.Sprintf("treaty_exists = $%d", argIdx))
		args = append(args, *in.TreatyExists)
		argIdx++
	}

	if in.TreatySummary != nil {
		sets = append(sets, fmt.Sprintf("treaty_summary = $%d", argIdx))
		args = append(args, *in.TreatySummary)
		argIdx++
	}

	if in.TreatySourceURL != nil {
		sets = append(sets, fmt.Sprintf("treaty_source_url = $%d", argIdx))
		args = append(args, *in.TreatySourceURL)
		argIdx++
	}

	if len(sets) == 0 {
		return s.getTreatyByID(ctx, id)
	}

	sets = append(sets, "updated_at = now()")

	q := fmt.Sprintf(`
UPDATE tax_treaty_pairs
SET %s
WHERE id = $%d
`, strings.Join(sets, ", "), argIdx)

	args = append(args, id)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return TreatyRow{}, fmt.Errorf("taxcalculator: update treaty: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return TreatyRow{}, pgx.ErrNoRows
	}

	return s.getTreatyByID(ctx, id)
}

// DeleteTreaty deletes a tax_treaty_pairs row by id and .
func (s *Store) DeleteTreaty(ctx context.Context, id pgtype.UUID) error {
	const q = `
DELETE FROM tax_treaty_pairs
WHERE id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("taxcalculator: delete treaty: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// AdminListFilter holds filter parameters for the admin guide list.
type AdminListFilter struct {
	Status             string      // optional
	EmploymentStatus   string      // optional
	DestinationSpaceID pgtype.UUID // optional
	OriginSpaceID      pgtype.UUID // optional
	Q                  string      // optional: search summary/obligations
	Limit              int32
	Offset             int32
}

// AdminList returns all guides (any status) for the admin management view.
func (s *Store) AdminList(ctx context.Context, f AdminListFilter) ([]GuideRow, []TreatyRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("g.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if f.EmploymentStatus != "" {
		conditions = append(conditions, fmt.Sprintf("g.employment_status = $%d", argIdx))
		args = append(args, f.EmploymentStatus)
		argIdx++
	}

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("g.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.OriginSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("g.origin_space_id = $%d", argIdx))
		args = append(args, f.OriginSpaceID)
		argIdx++
	}

	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("(g.residency_trigger ILIKE $%d OR g.first_year_obligations ILIKE $%d)", argIdx, argIdx))
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

	listQuery := fmt.Sprintf(`
SELECT g.id, g.origin_space_id, os.name AS origin_space_name,
       g.destination_space_id, ds.name AS destination_space_name,
       g.employment_status, g.residency_trigger, g.worldwide_income_taxed,
       g.first_year_obligations, g.action_checklist,
       g.status, g.is_active, g.ai_generated, g.ai_model, g.ai_prompt_hash,
       g.hitl_reviewed_by, g.hitl_reviewed_at, g.hitl_review_notes,
       g.created_at, g.updated_at
FROM tax_obligation_guides g
LEFT JOIN spaces os ON os.id = g.origin_space_id
LEFT JOIN spaces ds ON ds.id = g.destination_space_id
WHERE %s
ORDER BY g.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("taxcalculator: admin list guides: %w", err)
	}
	defer rows.Close()

	var out []GuideRow

	for rows.Next() {
		var r GuideRow
		if err := rows.Scan(
			&r.ID, &r.OriginSpaceID, &r.OriginSpaceName,
			&r.DestinationSpaceID, &r.DestinationSpaceName,
			&r.EmploymentStatus, &r.ResidencyTrigger, &r.WorldwideIncomeTaxed,
			&r.FirstYearObligations, &r.ActionChecklist,
			&r.Status, &r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
			&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, nil, 0, fmt.Errorf("taxcalculator: admin scan guide row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, 0, fmt.Errorf("taxcalculator: admin iterate guide rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM tax_obligation_guides g
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, nil, 0, fmt.Errorf("taxcalculator: admin count guides: %w", err)
	}

	// Fetch all treaty pairs for the tenant so the admin view can show
	// them alongside guides without a second paginated request.
	treatyQuery := `
SELECT t.id, t.origin_space_id, os.name AS origin_space_name,
       t.destination_space_id, ds.name AS destination_space_name,
       t.treaty_exists, t.treaty_summary, t.treaty_source_url,
       t.created_at, t.updated_at
FROM tax_treaty_pairs t
LEFT JOIN spaces os ON os.id = t.origin_space_id
LEFT JOIN spaces ds ON ds.id = t.destination_space_id

ORDER BY t.updated_at DESC
`

	tRows, err := s.pool.Reader.Query(ctx, treatyQuery, (pgtype.UUID{}))
	if err != nil {
		return nil, nil, 0, fmt.Errorf("taxcalculator: admin list treaties: %w", err)
	}

	defer tRows.Close()

	var treaties []TreatyRow

	for tRows.Next() {
		var r TreatyRow
		if err := tRows.Scan(
			&r.ID, &r.OriginSpaceID, &r.OriginSpaceName,
			&r.DestinationSpaceID, &r.DestinationSpaceName,
			&r.TreatyExists, &r.TreatySummary, &r.TreatySourceURL,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, nil, 0, fmt.Errorf("taxcalculator: admin scan treaty row: %w", err)
		}

		treaties = append(treaties, r)
	}

	if err := tRows.Err(); err != nil {
		return nil, nil, 0, fmt.Errorf("taxcalculator: admin iterate treaty rows: %w", err)
	}

	return out, treaties, total, nil
}
