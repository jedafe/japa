// Package crscalculator implements the Express Entry CRS Calculator
// (§107, ADR-118).
//
// The package provides versioned CRS scoring tables (crs_rulesets +
// crs_factor_points), draw history (crs_draws), and authenticated score
// history (crs_score_history). Point values are data — the engine in
// service.go interprets factor rows against a submitted profile, so an
// IRCC rules change lands as a new ruleset, not a code deploy.
// // Ref: japaconnect-engineering/sections/107-crs-calculator.md
// //	japaconnect-engineering/adr/ADR-118-crs-calculator.md
package crscalculator

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Factor groups (§107).
const (
	GroupCore            = "core"
	GroupSpouse          = "spouse"
	GroupTransferability = "transferability"
	GroupAdditional      = "additional"
)

// Ruleset statuses.
const (
	RulesetDraft      = "draft"
	RulesetActive     = "active"
	RulesetSuperseded = "superseded"
)

// Draw categories.
const (
	DrawGeneral     = "general"
	DrawCEC         = "cec"
	DrawPNP         = "pnp"
	DrawFrench      = "french"
	DrawSTEM        = "stem"
	DrawHealth      = "health"
	DrawTrades      = "trades"
	DrawAgriculture = "agriculture"
	DrawEducation   = "education"
	DrawTransport   = "transport"
)

// Store is the data-access layer for the crs_* tables. Direct pgx
// queries — no sqlc-generated queries for these tables yet. No business
// logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a CRS calculator store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// RulesetRow is a single crs_rulesets row.
type RulesetRow struct {
	ID            pgtype.UUID
	VersionLabel  string
	EffectiveDate pgtype.Date
	Status        string
	SourceURL     *string
}

// FactorRow is a single crs_factor_points row.
type FactorRow struct {
	ID               pgtype.UUID
	RulesetID        pgtype.UUID
	FactorGroup      string
	FactorKey        string
	InputDescriptor  []byte // jsonb
	Label            string
	PointsSingle     int32
	PointsWithSpouse pgtype.Int4
	SortOrder        int32
}

// DrawRow is a single crs_draws row.
type DrawRow struct {
	ID                pgtype.UUID
	DrawNumber        pgtype.Int4
	DrawDate          pgtype.Date
	Category          string
	CRSCutoff         int32
	InvitationsIssued int32
	TieBreakRule      *string
	SourceURL         *string
	Status            string
}

// ScoreHistoryRow is a single crs_score_history row.
type ScoreHistoryRow struct {
	ID        pgtype.UUID
	UserID    pgtype.UUID
	RulesetID pgtype.UUID
	Inputs    []byte
	Score     int32
	Breakdown []byte
	CreatedAt pgtype.Timestamptz
}

// GetActiveRuleset returns the most recent active ruleset.
func (s *Store) GetActiveRuleset(ctx context.Context) (RulesetRow, error) {
	var r RulesetRow
	err := s.pool.Reader.QueryRow(ctx, `
		SELECT id, version_label, effective_date, status, source_url
		FROM crs_rulesets
		WHERE status = 'active'
		ORDER BY effective_date DESC
		LIMIT 1`).Scan(&r.ID, &r.VersionLabel, &r.EffectiveDate, &r.Status, &r.SourceURL)
	return r, err
}

// GetRulesetByVersion returns a ruleset by its version label.
func (s *Store) GetRulesetByVersion(ctx context.Context, version string) (RulesetRow, error) {
	var r RulesetRow
	err := s.pool.Reader.QueryRow(ctx, `
		SELECT id, version_label, effective_date, status, source_url
		FROM crs_rulesets
		WHERE version_label = $1`, version).
		Scan(&r.ID, &r.VersionLabel, &r.EffectiveDate, &r.Status, &r.SourceURL)
	return r, err
}

// ListRulesets returns all rulesets (admin).
func (s *Store) ListRulesets(ctx context.Context) ([]RulesetRow, error) {
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT id, version_label, effective_date, status, source_url
		FROM crs_rulesets
		ORDER BY effective_date DESC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RulesetRow
	for rows.Next() {
		var r RulesetRow
		if err := rows.Scan(&r.ID, &r.VersionLabel, &r.EffectiveDate, &r.Status, &r.SourceURL); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListFactorPoints returns all factor rows for a ruleset, ordered.
func (s *Store) ListFactorPoints(ctx context.Context, rulesetID pgtype.UUID) ([]FactorRow, error) {
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT id, ruleset_id, factor_group, factor_key, input_descriptor,
		       label, points_single, points_with_spouse, sort_order
		FROM crs_factor_points
		WHERE ruleset_id = $1
		ORDER BY factor_group, factor_key, sort_order`, rulesetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FactorRow
	for rows.Next() {
		var f FactorRow
		if err := rows.Scan(&f.ID, &f.RulesetID, &f.FactorGroup, &f.FactorKey,
			&f.InputDescriptor, &f.Label, &f.PointsSingle, &f.PointsWithSpouse, &f.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListDraws returns published draws, optionally filtered by category,
// newest first.
func (s *Store) ListDraws(ctx context.Context, category string, limit int32) ([]DrawRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	q := `
		SELECT id, draw_number, draw_date, category, crs_cutoff,
		       invitations_issued, tie_break_rule, source_url, status
		FROM crs_draws
		WHERE status = 'published'`
	args := []any{limit}
	if category != "" {
		q += ` AND category = $2`
		args = append(args, category)
	}
	q += ` ORDER BY draw_date DESC LIMIT $1`

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanDraws(rows)
}

// LatestDraws returns the most recent published draw per category.
func (s *Store) LatestDraws(ctx context.Context) ([]DrawRow, error) {
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT DISTINCT ON (category)
		       id, draw_number, draw_date, category, crs_cutoff,
		       invitations_issued, tie_break_rule, source_url, status
		FROM crs_draws
		WHERE status = 'published'
		ORDER BY category, draw_date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanDraws(rows)
}

func scanDraws(rows pgx.Rows) ([]DrawRow, error) {
	var out []DrawRow
	for rows.Next() {
		var d DrawRow
		if err := rows.Scan(&d.ID, &d.DrawNumber, &d.DrawDate, &d.Category,
			&d.CRSCutoff, &d.InvitationsIssued, &d.TieBreakRule, &d.SourceURL, &d.Status); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SaveScoreHistory inserts a score-history row for an authenticated user.
func (s *Store) SaveScoreHistory(ctx context.Context, userID, rulesetID pgtype.UUID, inputs, breakdown []byte, score int32) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO crs_score_history (user_id, ruleset_id, inputs, score, breakdown)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`, userID, rulesetID, inputs, score, breakdown).Scan(&id)
	return id, err
}

// ListScoreHistory returns a user's score history, newest first.
func (s *Store) ListScoreHistory(ctx context.Context, userID pgtype.UUID, page, limit int32) ([]ScoreHistoryRow, int64, error) {
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, `
		SELECT count(*) FROM crs_score_history
		WHERE user_id = $1 AND deleted_at IS NULL`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Reader.Query(ctx, `
		SELECT id, user_id, ruleset_id, inputs, score, breakdown, created_at
		FROM crs_score_history
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, userID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []ScoreHistoryRow
	for rows.Next() {
		var h ScoreHistoryRow
		if err := rows.Scan(&h.ID, &h.UserID, &h.RulesetID, &h.Inputs, &h.Score, &h.Breakdown, &h.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, h)
	}
	return out, total, rows.Err()
}

// CreateRuleset inserts a new ruleset in draft status.
func (s *Store) CreateRuleset(ctx context.Context, tx pgx.Tx, versionLabel string, effectiveDate pgtype.Date, sourceURL string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO crs_rulesets (version_label, effective_date, status, source_url)
		VALUES ($1, $2, 'draft', NULLIF($3, ''))
		RETURNING id`, versionLabel, effectiveDate, sourceURL).Scan(&id)
	return id, err
}

// CreateFactorPoint inserts a factor row under a ruleset.
func (s *Store) CreateFactorPoint(ctx context.Context, tx pgx.Tx, f FactorRow) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO crs_factor_points
		  (ruleset_id, factor_group, factor_key, input_descriptor, label,
		   points_single, points_with_spouse, sort_order)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		f.RulesetID, f.FactorGroup, f.FactorKey, f.InputDescriptor, f.Label,
		f.PointsSingle, f.PointsWithSpouse, f.SortOrder)
	return err
}

// ApproveRuleset marks a ruleset active and supersedes the prior active
// one, atomically.
func (s *Store) ApproveRuleset(ctx context.Context, id, reviewerID pgtype.UUID) error {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE crs_rulesets SET status = 'superseded', updated_at = now()
		WHERE status = 'active'`); err != nil {
		return err
	}

	res, err := tx.Exec(ctx, `
		UPDATE crs_rulesets
		SET status = 'active', hitl_reviewed_by = $2, hitl_reviewed_at = now(), updated_at = now()
		WHERE id = $1`, id, reviewerID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return tx.Commit(ctx)
}

// CreateDraw inserts a draw entry.
func (s *Store) CreateDraw(ctx context.Context, d DrawRow) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO crs_draws
		  (draw_number, draw_date, category, crs_cutoff, invitations_issued,
		   tie_break_rule, source_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`, d.DrawNumber, d.DrawDate, d.Category, d.CRSCutoff,
		d.InvitationsIssued, d.TieBreakRule, d.SourceURL).Scan(&id)
	return id, err
}

// UpdateDraw updates mutable fields of a draw entry.
func (s *Store) UpdateDraw(ctx context.Context, id pgtype.UUID, d DrawRow) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE crs_draws
		SET draw_number = COALESCE($2, draw_number),
		    draw_date = COALESCE($3, draw_date),
		    category = COALESCE($4, category),
		    crs_cutoff = COALESCE($5, crs_cutoff),
		    invitations_issued = COALESCE($6, invitations_issued),
		    tie_break_rule = COALESCE($7, tie_break_rule),
		    source_url = COALESCE($8, source_url),
		    updated_at = now()
		WHERE id = $1`, id, d.DrawNumber, d.DrawDate, d.Category, d.CRSCutoff,
		d.InvitationsIssued, d.TieBreakRule, d.SourceURL)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// BeginTx exposes a transaction for multi-row admin writes.
func (s *Store) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Writer.Begin(ctx)
}

// RejectRuleset marks a draft ruleset rejected (HITL reject path).
func (s *Store) RejectRuleset(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE crs_rulesets
		SET status = 'rejected', hitl_reviewed_by = $2, hitl_reviewed_at = now(),
		    hitl_review_notes = NULLIF($3,''), updated_at = now()
		WHERE id = $1`, id, reviewerID, notes)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
