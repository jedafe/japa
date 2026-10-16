// Package ecaguide implements the Educational Credential Assessment
// (ECA) Guide (§109, ADR-120).
//
// eca_providers is a global lookup (WES, IQAS, ICAS, CES, ECCTIS…);
// eca_guides holds destination × provider × purpose step-by-step guides;
// eca_community_updates carries member-submitted fee/timeline reports
// through a moderation queue.
// // Ref: japaconnect-engineering/sections/109-eca-guide.md
// //	japaconnect-engineering/adr/ADR-120-eca-guide.md
package ecaguide

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Guide/update statuses.
const (
	StatusDraft     = "draft"
	StatusAIDraft   = "ai_draft"
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
)

// Store is the data-access layer for eca_providers, eca_guides and
// eca_community_updates. Direct pgx — no sqlc queries for these tables
// yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates an ECA store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// ProviderRow is a single eca_providers row.
type ProviderRow struct {
	ID       pgtype.UUID
	Slug     string
	Name     string
	Website  *string
	IsActive bool
}

// GuideRow is an eca_guides row joined to provider + destination names.
type GuideRow struct {
	ID                   pgtype.UUID
	DestinationSpaceID   pgtype.UUID
	DestinationSpaceName *string
	ProviderID           pgtype.UUID
	ProviderSlug         *string
	ProviderName         *string
	Purpose              string
	PathwaySteps         []byte // jsonb
	FeeEstimate          *string
	Currency             *string
	TimelineEstimate     *string
	DocumentsRequired    []byte // jsonb
	AcceptedFor          []byte // jsonb
	OfficialURL          *string
	SourceURLLastChecked pgtype.Date
	Status               string
	IsActive             pgtype.Bool
	AIGenerated          bool
	HITLReviewedAt       pgtype.Timestamptz
	IsJapabotIndexed     bool
	CreatedAt            pgtype.Timestamptz
	UpdatedAt            pgtype.Timestamptz
}

// UpdateRow is a single eca_community_updates row.
type UpdateRow struct {
	ID             pgtype.UUID
	GuideID        pgtype.UUID
	UserID         pgtype.UUID
	UserName       *string
	StepIndex      pgtype.Int4
	FeeEstimate    *string
	TurnaroundDays pgtype.Int4
	Note           *string
	Status         string
	CreatedAt      pgtype.Timestamptz
}

const guideSelect = `
	SELECT g.id, g.destination_space_id, s.name,
	       g.provider_id, p.slug, p.name,
	       g.purpose, g.pathway_steps, g.fee_estimate, g.currency,
	       g.timeline_estimate, g.documents_required, g.accepted_for,
	       g.official_url, g.source_url_last_checked, g.status, g.is_active,
	       g.ai_generated, g.hitl_reviewed_at, g.is_japabot_indexed,
	       g.created_at, g.updated_at
	FROM eca_guides g
	LEFT JOIN spaces s ON s.id = g.destination_space_id
	LEFT JOIN eca_providers p ON p.id = g.provider_id`

func scanGuides(rows pgx.Rows) ([]GuideRow, error) {
	var out []GuideRow

	for rows.Next() {
		var g GuideRow
		if err := rows.Scan(&g.ID, &g.DestinationSpaceID, &g.DestinationSpaceName,
			&g.ProviderID, &g.ProviderSlug, &g.ProviderName,
			&g.Purpose, &g.PathwaySteps, &g.FeeEstimate, &g.Currency,
			&g.TimelineEstimate, &g.DocumentsRequired, &g.AcceptedFor,
			&g.OfficialURL, &g.SourceURLLastChecked, &g.Status, &g.IsActive,
			&g.AIGenerated, &g.HITLReviewedAt, &g.IsJapabotIndexed,
			&g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}

		out = append(out, g)
	}

	return out, rows.Err()
}

// ListProviders returns active providers, optionally filtered to those
// with a published guide at a destination.
func (s *Store) ListProviders(ctx context.Context, destSpaceID pgtype.UUID) ([]ProviderRow, error) {
	q := `SELECT id, slug, name, website, is_active FROM eca_providers WHERE is_active`

	args := []any{}
	if destSpaceID.Valid {
		args = append(args, destSpaceID)
		q += ` AND id IN (SELECT provider_id FROM eca_guides
		        WHERE destination_space_id = $1 AND status = 'published' AND is_active)`
	}

	q += ` ORDER BY name`

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProviderRow

	for rows.Next() {
		var p ProviderRow
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.Website, &p.IsActive); err != nil {
			return nil, err
		}

		out = append(out, p)
	}

	return out, rows.Err()
}

// ListGuides returns published guides filtered by dest/purpose/provider.
// The provider filter accepts either the provider slug ("wes") or the
// provider UUID — both public surfaces send the UUID.
func (s *Store) ListGuides(ctx context.Context, destSpaceID pgtype.UUID, purpose, provider string) ([]GuideRow, error) {
	q := guideSelect + ` WHERE g.status = 'published' AND g.is_active`
	args := []any{}

	add := func(cond string, v any) {
		args = append(args, v)
		q += ` AND ` + cond + ` = $` + itoa(len(args))
	}
	if destSpaceID.Valid {
		add(`g.destination_space_id`, destSpaceID)
	}

	if purpose != "" {
		add(`g.purpose`, purpose)
	}

	if provider != "" {
		var pid pgtype.UUID
		if err := pid.Scan(provider); err == nil && pid.Valid {
			add(`g.provider_id`, pid)
		} else {
			add(`p.slug`, provider)
		}
	}

	q += ` ORDER BY s.name, p.name`

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return scanGuides(rows)
}

// GetGuide returns one published guide by id. Draft/rejected guides are
// admin-surface only — the public detail route must not expose them.
func (s *Store) GetGuide(ctx context.Context, id pgtype.UUID) (GuideRow, error) {
	rows, err := s.pool.Reader.Query(ctx, guideSelect+` WHERE g.id = $1 AND g.status = 'published' AND g.is_active`, id)
	if err != nil {
		return GuideRow{}, err
	}

	defer rows.Close()

	guides, err := scanGuides(rows)
	if err != nil {
		return GuideRow{}, err
	}

	if len(guides) == 0 {
		return GuideRow{}, pgx.ErrNoRows
	}

	return guides[0], nil
}

// CompareGuides returns published guides for a destination across a set
// of provider ids (side-by-side comparison).
func (s *Store) CompareGuides(ctx context.Context, destSpaceID pgtype.UUID, providerIDs []pgtype.UUID) ([]GuideRow, error) {
	rows, err := s.pool.Reader.Query(ctx,
		guideSelect+` WHERE g.destination_space_id = $1 AND g.provider_id = ANY($2)
		              AND g.status = 'published' AND g.is_active
		              ORDER BY p.name, g.purpose`, destSpaceID, providerIDs)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return scanGuides(rows)
}

// ListApprovedUpdates returns approved community updates for a guide.
func (s *Store) ListApprovedUpdates(ctx context.Context, guideID pgtype.UUID) ([]UpdateRow, error) {
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT u.id, u.guide_id, u.user_id, up.display_name,
		       u.step_index, u.fee_estimate, u.turnaround_days, u.note, u.status, u.created_at
		FROM eca_community_updates u
		LEFT JOIN user_profiles up ON up.user_id = u.user_id
		WHERE u.guide_id = $1 AND u.status = 'approved' AND u.deleted_at IS NULL
		ORDER BY u.created_at DESC`, guideID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return scanUpdates(rows)
}

func scanUpdates(rows pgx.Rows) ([]UpdateRow, error) {
	var out []UpdateRow

	for rows.Next() {
		var u UpdateRow
		if err := rows.Scan(&u.ID, &u.GuideID, &u.UserID, &u.UserName,
			&u.StepIndex, &u.FeeEstimate, &u.TurnaroundDays, &u.Note, &u.Status, &u.CreatedAt); err != nil {
			return nil, err
		}

		out = append(out, u)
	}

	return out, rows.Err()
}

// CreateCommunityUpdate inserts a pending community update (auth member).
// The guide must be published — otherwise pgx.ErrNoRows.
func (s *Store) CreateCommunityUpdate(ctx context.Context, guideID, userID pgtype.UUID, stepIndex *int32, feeEstimate string, turnaroundDays *int32, note string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO eca_community_updates
		  (guide_id, user_id, step_index, fee_estimate, turnaround_days, note)
		SELECT $1, $2, $3, NULLIF($4,''), $5, NULLIF($6,'')
		WHERE EXISTS (
			SELECT 1 FROM eca_guides g
			WHERE g.id = $1 AND g.status = 'published' AND g.is_active)
		RETURNING id`, guideID, userID, stepIndex, feeEstimate, turnaroundDays, note).Scan(&id)

	return id, err
}

// AdminListGuides returns guides in any status (admin).
func (s *Store) AdminListGuides(ctx context.Context, destSpaceID pgtype.UUID, status string, page, limit int32) ([]GuideRow, int64, error) {
	where := ` WHERE true`

	args := []any{}
	if destSpaceID.Valid {
		args = append(args, destSpaceID)
		where += ` AND g.destination_space_id = $` + itoa(len(args))
	}

	if status != "" {
		args = append(args, status)
		where += ` AND g.status = $` + itoa(len(args))
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx, `SELECT count(*) FROM eca_guides g`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, (page-1)*limit)

	rows, err := s.pool.Reader.Query(ctx,
		guideSelect+where+` ORDER BY g.updated_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}

	defer rows.Close()

	guides, err := scanGuides(rows)

	return guides, total, err
}

// CreateGuide inserts a guide (admin).
func (s *Store) CreateGuide(ctx context.Context, g GuideRow) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO eca_guides
		  (destination_space_id, provider_id, purpose, pathway_steps,
		   fee_estimate, currency, timeline_estimate, documents_required,
		   accepted_for, official_url, status, ai_generated)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),$11,$12)
		RETURNING id`,
		g.DestinationSpaceID, g.ProviderID, g.Purpose, g.PathwaySteps,
		g.FeeEstimate, g.Currency, g.TimelineEstimate, g.DocumentsRequired,
		g.AcceptedFor, strOrEmpty(g.OfficialURL), g.Status, g.AIGenerated).Scan(&id)

	return id, err
}

// UpdateGuide applies a partial update (admin).
func (s *Store) UpdateGuide(ctx context.Context, id pgtype.UUID, g GuideRow) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE eca_guides SET
		  purpose = COALESCE(NULLIF($2,''), purpose),
		  pathway_steps = COALESCE($3, pathway_steps),
		  fee_estimate = COALESCE($4, fee_estimate),
		  currency = COALESCE($5, currency),
		  timeline_estimate = COALESCE($6, timeline_estimate),
		  documents_required = COALESCE($7, documents_required),
		  accepted_for = COALESCE($8, accepted_for),
		  official_url = COALESCE($9, official_url),
		  status = COALESCE(NULLIF($10,''), status),
		  is_active = COALESCE($11, is_active),
		  updated_at = now()
		WHERE id = $1`,
		id, g.Purpose, nullBytes(g.PathwaySteps), g.FeeEstimate, g.Currency,
		g.TimelineEstimate, nullBytes(g.DocumentsRequired), nullBytes(g.AcceptedFor),
		g.OfficialURL, g.Status, g.IsActive)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ApproveGuide publishes a guide (HITL).
func (s *Store) ApproveGuide(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE eca_guides
		SET status = 'published', hitl_reviewed_by = $2, hitl_reviewed_at = now(),
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

// AdminListUpdates returns community updates for moderation (admin).
func (s *Store) AdminListUpdates(ctx context.Context, status string, page, limit int32) ([]UpdateRow, int64, error) {
	where := ` WHERE u.deleted_at IS NULL`

	args := []any{}
	if status != "" {
		args = append(args, status)
		where += ` AND u.status = $` + itoa(len(args))
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx,
		`SELECT count(*) FROM eca_community_updates u`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, (page-1)*limit)

	rows, err := s.pool.Reader.Query(ctx, `
		SELECT u.id, u.guide_id, u.user_id, up.display_name,
		       u.step_index, u.fee_estimate, u.turnaround_days, u.note, u.status, u.created_at
		FROM eca_community_updates u
		LEFT JOIN user_profiles up ON up.user_id = u.user_id`+
		where+` ORDER BY u.created_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}

	defer rows.Close()

	updates, err := scanUpdates(rows)

	return updates, total, err
}

// ModerateUpdate approves or rejects a community update (admin).
func (s *Store) ModerateUpdate(ctx context.Context, id, reviewerID pgtype.UUID, status string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE eca_community_updates
		SET status = $2, reviewed_by = $3
		WHERE id = $1 AND deleted_at IS NULL`, id, status, reviewerID)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// helpers

func itoa(i int) string {
	if i < 1 {
		return "1"
	}

	var buf [4]byte

	p := len(buf)
	for i > 0 {
		p--
		buf[p] = byte('0' + i%10)
		i /= 10
	}

	return string(buf[p:])
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

// RejectGuide marks a pending/draft guide rejected (HITL).
func (s *Store) RejectGuide(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE eca_guides
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

// DeleteGuide removes a non-published guide. Published guides are
// protected — deactivate them instead (is_active=false).
func (s *Store) DeleteGuide(ctx context.Context, id pgtype.UUID) error {
	res, err := s.pool.Writer.Exec(ctx, `
		DELETE FROM eca_guides
		WHERE id = $1 AND status IN ('draft','ai_draft','pending','rejected')`, id)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// AdminListProviders returns all providers including inactive (admin).
func (s *Store) AdminListProviders(ctx context.Context) ([]ProviderRow, error) {
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT id, slug, name, website, is_active
		FROM eca_providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProviderRow

	for rows.Next() {
		var p ProviderRow
		if err := rows.Scan(&p.ID, &p.Slug, &p.Name, &p.Website, &p.IsActive); err != nil {
			return nil, err
		}

		out = append(out, p)
	}

	return out, rows.Err()
}

// CreateProvider inserts a provider (admin).
func (s *Store) CreateProvider(ctx context.Context, slug, name string, website *string, isActive pgtype.Bool) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO eca_providers (slug, name, website, is_active)
		VALUES ($1, $2, $3, COALESCE($4, true))
		RETURNING id`, slug, name, website, isActive).Scan(&id)

	return id, err
}

// UpdateProvider applies a partial update (admin). Empty strings and
// invalid pgtype values leave the column unchanged.
func (s *Store) UpdateProvider(ctx context.Context, id pgtype.UUID, slug, name string, website *string, isActive pgtype.Bool) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE eca_providers SET
		  slug = COALESCE(NULLIF($2,''), slug),
		  name = COALESCE(NULLIF($3,''), name),
		  website = COALESCE($4, website),
		  is_active = COALESCE($5, is_active),
		  updated_at = now()
		WHERE id = $1`, id, slug, name, website, isActive)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// DeleteProvider removes a provider only when no guides reference it —
// eca_guides.provider_id is RESTRICT, so this returns an error rather
// than orphaning guides.
func (s *Store) DeleteProvider(ctx context.Context, id pgtype.UUID) error {
	res, err := s.pool.Writer.Exec(ctx, `
		DELETE FROM eca_providers WHERE id = $1
		AND NOT EXISTS (SELECT 1 FROM eca_guides WHERE provider_id = $1)`, id)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
