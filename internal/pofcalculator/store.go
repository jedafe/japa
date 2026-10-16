// Package pofcalculator implements the Proof of Funds (POF) Calculator
// (§108, ADR-119).
//
// pof_requirements holds admin-curated settlement-fund schedules keyed by
// destination space × visa type. Amount schedules are data (JSONB); the
// service resolves household size → required amount and converts to
// NGN/USD via the shared §74 remittance FX service.
// // Ref: japaconnect-engineering/sections/108-pof-calculator.md
// //	japaconnect-engineering/adr/ADR-119-pof-calculator.md
package pofcalculator

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Requirement statuses.
const (
	StatusDraft     = "draft"
	StatusAIDraft   = "ai_draft"
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

// Store is the data-access layer for pof_requirements. Direct pgx — no
// sqlc-generated queries for this table yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a POF store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// RequirementRow is a single pof_requirements row joined to its
// destination space name.
type RequirementRow struct {
	ID                   pgtype.UUID
	DestinationSpaceID   pgtype.UUID
	DestinationSpaceName *string
	DestinationSpaceCode *string
	VisaType             string
	ProgramLabel         string
	AmountSchedule       []byte // jsonb
	Currency             string
	FundsMustBeLiquid    pgtype.Bool
	AcceptedProof        []byte // jsonb
	Notes                *string
	EffectiveDate        pgtype.Date
	SourceURL            *string
	SourceURLLastChecked pgtype.Date
	Status               string
	IsActive             pgtype.Bool
	AIGenerated          bool
	HITLReviewedAt       pgtype.Timestamptz
	IsJapabotIndexed     bool
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
}

const reqSelect = `
	SELECT p.id, p.destination_space_id, s.name, s.geo_code,
	       p.visa_type, p.program_label, p.amount_schedule, p.currency,
	       p.funds_must_be_liquid, p.accepted_proof, p.notes,
	       p.effective_date, p.source_url, p.source_url_last_checked,
	       p.status, p.is_active, p.ai_generated, p.hitl_reviewed_at,
	       p.is_japabot_indexed, p.created_at, p.updated_at
	FROM pof_requirements p
	LEFT JOIN spaces s ON s.id = p.destination_space_id`

func scanRequirements(rows pgx.Rows) ([]RequirementRow, error) {
	var out []RequirementRow

	for rows.Next() {
		var r RequirementRow
		if err := rows.Scan(&r.ID, &r.DestinationSpaceID, &r.DestinationSpaceName, &r.DestinationSpaceCode,
			&r.VisaType, &r.ProgramLabel, &r.AmountSchedule, &r.Currency,
			&r.FundsMustBeLiquid, &r.AcceptedProof, &r.Notes,
			&r.EffectiveDate, &r.SourceURL, &r.SourceURLLastChecked,
			&r.Status, &r.IsActive, &r.AIGenerated, &r.HITLReviewedAt,
			&r.IsJapabotIndexed, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}

		out = append(out, r)
	}

	return out, rows.Err()
}

// ListRequirements returns published, active requirements filtered by
// destination and/or visa type. Public.
func (s *Store) ListRequirements(ctx context.Context, destSpaceID pgtype.UUID, visaType string) ([]RequirementRow, error) {
	q := reqSelect + ` WHERE p.status = 'published' AND p.is_active`

	args := []any{}
	if destSpaceID.Valid {
		args = append(args, destSpaceID)
		q += ` AND p.destination_space_id = $` + n(len(args))
	}

	if visaType != "" {
		args = append(args, visaType)
		q += ` AND p.visa_type = $` + n(len(args))
	}

	q += ` ORDER BY s.name, p.visa_type, p.program_label`

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return scanRequirements(rows)
}

// DistinctVisaTypes returns the visa types present for a destination —
// filter options for the UI (Rule 16: options come from the API).
func (s *Store) DistinctVisaTypes(ctx context.Context, destSpaceID pgtype.UUID) ([]string, error) {
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT DISTINCT visa_type FROM pof_requirements
		WHERE status = 'published' AND is_active AND destination_space_id = $1
		ORDER BY visa_type`, destSpaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string

	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}

		out = append(out, v)
	}

	return out, rows.Err()
}

// AdminListRequirements returns requirements in any status (admin).
// q matches program_label/visa_type (ILIKE); sort+order select a
// whitelisted column — unknown keys fall back to updated_at DESC.
func (s *Store) AdminListRequirements(ctx context.Context, destSpaceID pgtype.UUID, visaType, status, q, sort, order string, page, limit int32) ([]RequirementRow, int64, error) {
	where := ` WHERE true`
	args := []any{}

	add := func(cond string, v any) {
		args = append(args, v)
		where += ` AND ` + cond + ` = $` + n(len(args))
	}
	if destSpaceID.Valid {
		add(`p.destination_space_id`, destSpaceID)
	}

	if visaType != "" {
		add(`p.visa_type`, visaType)
	}

	if status != "" {
		add(`p.status`, status)
	}

	if q != "" {
		args = append(args, q)
		where += ` AND (p.program_label ILIKE '%' || $` + n(len(args)) + ` || '%' OR p.visa_type ILIKE '%' || $` + n(len(args)) + ` || '%')`
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx,
		`SELECT count(*) FROM pof_requirements p`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Sort: whitelist of allowed columns to prevent SQL injection via
	// user-supplied sort param (geo-aware-tool-pattern.md admin list).
	orderClause := "p.updated_at DESC"

	if col, ok := adminSortColumn(sort); ok {
		dir := "DESC"
		if strings.EqualFold(order, "asc") {
			dir = "ASC"
		}

		orderClause = col + " " + dir
	}

	args = append(args, limit, (page-1)*limit)

	rows, err := s.pool.Reader.Query(ctx,
		reqSelect+where+` ORDER BY `+orderClause+` LIMIT $`+n(len(args)-1)+` OFFSET $`+n(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}

	defer rows.Close()

	reqs, err := scanRequirements(rows)

	return reqs, total, err
}

// adminSortColumn maps a user-supplied sort key to a qualified column
// expression. Returns false for unknown columns to prevent SQL injection
// via the sort query param (geo-aware-tool-pattern.md admin list pattern).
func adminSortColumn(key string) (string, bool) {
	switch strings.ToLower(key) {
	case "destination", "destination_space_name":
		return "s.name", true
	case "visa_type":
		return "p.visa_type", true
	case "program_label":
		return "p.program_label", true
	case "currency":
		return "p.currency", true
	case "effective_date":
		return "p.effective_date", true
	case "status":
		return "p.status", true
	case "created_at":
		return "p.created_at", true
	case "updated_at":
		return "p.updated_at", true
	}

	return "", false
}

// CreateRequirement inserts a requirement (admin).
func (s *Store) CreateRequirement(ctx context.Context, r RequirementRow) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO pof_requirements
		  (destination_space_id, visa_type, program_label, amount_schedule,
		   currency, funds_must_be_liquid, accepted_proof, notes,
		   effective_date, source_url, status, ai_generated)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,$12)
		RETURNING id`,
		r.DestinationSpaceID, r.VisaType, r.ProgramLabel, r.AmountSchedule,
		r.Currency, r.FundsMustBeLiquid, r.AcceptedProof, r.Notes,
		r.EffectiveDate, strOrEmpty(r.SourceURL), r.Status, r.AIGenerated).Scan(&id)

	return id, err
}

// UpdateRequirement applies a partial update (admin). Only non-nil fields
// change; NULL JSONB fields are left untouched via COALESCE.
func (s *Store) UpdateRequirement(ctx context.Context, id pgtype.UUID, r RequirementRow) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE pof_requirements SET
		  destination_space_id = COALESCE($13, destination_space_id),
		  visa_type = COALESCE(NULLIF($2,''), visa_type),
		  program_label = COALESCE(NULLIF($3,''), program_label),
		  amount_schedule = COALESCE($4, amount_schedule),
		  currency = COALESCE(NULLIF($5,''), currency),
		  funds_must_be_liquid = COALESCE($6, funds_must_be_liquid),
		  accepted_proof = COALESCE($7, accepted_proof),
		  notes = COALESCE($8, notes),
		  effective_date = COALESCE($9, effective_date),
		  source_url = COALESCE($10, source_url),
		  status = COALESCE(NULLIF($11,''), status),
		  is_active = COALESCE($12, is_active),
		  updated_at = now()
		WHERE id = $1`,
		id, r.VisaType, r.ProgramLabel, nullBytes(r.AmountSchedule),
		r.Currency, r.FundsMustBeLiquid, nullBytes(r.AcceptedProof),
		r.Notes, nullDate(r.EffectiveDate), r.SourceURL, r.Status, r.IsActive,
		nullUUID(r.DestinationSpaceID))
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ApproveRequirement publishes a requirement (HITL). Only rows awaiting
// review may be approved — re-approving published rows or resurrecting
// rejected ones is rejected by the status guard.
func (s *Store) ApproveRequirement(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE pof_requirements
		SET status = 'published', hitl_reviewed_by = $2, hitl_reviewed_at = now(),
		    hitl_review_notes = NULLIF($3,''), updated_at = now()
		WHERE id = $1 AND status IN ('draft','ai_draft','pending')`, id, reviewerID, notes)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// helpers — arg placeholders and nullable coercion

func n(i int) string {
	if i < 1 {
		i = 1
	}
	// positional params are small ints; format without strconv import churn
	digits := [4]byte{}

	p := len(digits)
	for i > 0 {
		p--
		digits[p] = byte('0' + i%10)
		i /= 10
	}

	return string(digits[p:])
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}

	return b
}

func nullDate(d pgtype.Date) any {
	if !d.Valid {
		return nil
	}

	return d
}

func nullUUID(u pgtype.UUID) any {
	if !u.Valid {
		return nil
	}

	return u
}

// GetRequirement returns a requirement by ID regardless of status
// (admin detail view).
func (s *Store) GetRequirement(ctx context.Context, id pgtype.UUID) (RequirementRow, error) {
	rows, err := s.pool.Reader.Query(ctx, reqSelect+` WHERE p.id = $1`, id)
	if err != nil {
		return RequirementRow{}, err
	}

	defer rows.Close()

	out, err := scanRequirements(rows)
	if err != nil {
		return RequirementRow{}, err
	}

	if len(out) == 0 {
		return RequirementRow{}, pgx.ErrNoRows
	}

	return out[0], nil
}

// DeleteRequirement removes a non-published requirement. Published rows
// are protected — they feed the public calculator.
func (s *Store) DeleteRequirement(ctx context.Context, id pgtype.UUID) error {
	res, err := s.pool.Writer.Exec(ctx, `
		DELETE FROM pof_requirements
		WHERE id = $1 AND status IN ('draft','ai_draft','pending','rejected')`, id)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// RejectRequirement marks a pending/draft requirement rejected (HITL).
// Published rows are protected — a live requirement is withdrawn via
// is_active, not rejected back into the review queue.
func (s *Store) RejectRequirement(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE pof_requirements
		SET status = 'rejected', hitl_reviewed_by = $2, hitl_reviewed_at = now(),
		    hitl_review_notes = NULLIF($3,''), updated_at = now()
		WHERE id = $1 AND status IN ('draft','ai_draft','pending')`, id, reviewerID, notes)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
