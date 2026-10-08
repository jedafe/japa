// Package sopgenerator implements the AI SOP Generator (§110, ADR-121).
//
// Three content tiers: sop_templates (admin-managed structure scaffolds),
// sop_samples (HITL-published reference SOPs, JapaBot-indexed), and
// sop_drafts (private user-owned generation output, soft-deleted). Draft
// generation is asynchronous via the sop:generate Asynq task.
// // Ref: japaconnect-engineering/sections/110-sop-generator.md
// //	japaconnect-engineering/adr/ADR-121-sop-generator.md
package sopgenerator

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Content statuses.
const (
	StatusDraft     = "draft"
	StatusAIDraft   = "ai_draft"
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

// Draft statuses.
const (
	DraftQueued    = "queued"
	DraftGenerated = "generated"
	DraftFailed    = "failed"
)

// Store is the data-access layer for the sop_* tables. Direct pgx — no
// sqlc queries for these tables yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates an SOP store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// TemplateRow is a single sop_templates row.
type TemplateRow struct {
	ID                 pgtype.UUID
	DestinationSpaceID pgtype.UUID
	Purpose            string
	Title              string
	Structure          []byte // jsonb
	Status             string
	IsActive           pgtype.Bool
	AIGenerated        bool
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

// SampleRow is a single sop_samples row.
type SampleRow struct {
	ID                 pgtype.UUID
	DestinationSpaceID pgtype.UUID
	DestinationName    *string
	ProgramLevel       string
	FieldOfStudy       *string
	Title              string
	Body               string
	WordCount          pgtype.Int4
	Status             string
	AIGenerated        bool
	IsJapabotIndexed   bool
	CreatedAt          pgtype.Timestamptz
}

// DraftRow is a single sop_drafts row.
type DraftRow struct {
	ID                 pgtype.UUID
	UserID             pgtype.UUID
	DestinationSpaceID pgtype.UUID
	Purpose            string
	Program            *string
	Institution        *string
	Title              *string
	Inputs             []byte // jsonb
	GeneratedText      *string
	Status             string
	AIModel            *string
	AIPromptHash       *string
	Version            int32
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

// ListTemplates returns published templates for a destination (plus
// generic NULL-destination templates) and purpose.
func (s *Store) ListTemplates(ctx context.Context, destSpaceID pgtype.UUID, purpose string) ([]TemplateRow, error) {
	q := `
		SELECT id, destination_space_id, purpose, title, structure, status, is_active,
		       ai_generated, created_at, updated_at
		FROM sop_templates
		WHERE status = 'published' AND is_active`
	args := []any{}
	if destSpaceID.Valid {
		args = append(args, destSpaceID)
		q += ` AND (destination_space_id = $1 OR destination_space_id IS NULL)`
	}
	if purpose != "" {
		args = append(args, purpose)
		q += ` AND purpose = $` + itoa(len(args))
	}
	q += ` ORDER BY title`

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TemplateRow
	for rows.Next() {
		var t TemplateRow
		if err := rows.Scan(&t.ID, &t.DestinationSpaceID, &t.Purpose, &t.Title,
			&t.Structure, &t.Status, &t.IsActive, &t.AIGenerated,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListSamples returns published samples filtered by dest/level/field.
func (s *Store) ListSamples(ctx context.Context, destSpaceID pgtype.UUID, level, field string, page, limit int32) ([]SampleRow, int64, error) {
	where := ` WHERE p.status = 'published'`
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where += ` AND ` + cond + ` = $` + itoa(len(args))
	}
	if destSpaceID.Valid {
		add(`p.destination_space_id`, destSpaceID)
	}
	if level != "" {
		add(`p.program_level`, level)
	}
	if field != "" {
		add(`p.field_of_study`, field)
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx,
		`SELECT count(*) FROM sop_samples p`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, (page-1)*limit)
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT p.id, p.destination_space_id, s.name, p.program_level,
		       p.field_of_study, p.title, p.body, p.word_count, p.status,
		       p.ai_generated, p.is_japabot_indexed, p.created_at
		FROM sop_samples p
		LEFT JOIN spaces s ON s.id = p.destination_space_id`+
		where+` ORDER BY p.created_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []SampleRow
	for rows.Next() {
		var r SampleRow
		if err := rows.Scan(&r.ID, &r.DestinationSpaceID, &r.DestinationName,
			&r.ProgramLevel, &r.FieldOfStudy, &r.Title, &r.Body, &r.WordCount,
			&r.Status, &r.AIGenerated, &r.IsJapabotIndexed, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// GetSample returns one published sample by id.
func (s *Store) GetSample(ctx context.Context, id pgtype.UUID) (SampleRow, error) {
	var r SampleRow
	err := s.pool.Reader.QueryRow(ctx, `
		SELECT p.id, p.destination_space_id, s.name, p.program_level,
		       p.field_of_study, p.title, p.body, p.word_count, p.status,
		       p.ai_generated, p.is_japabot_indexed, p.created_at
		FROM sop_samples p
		LEFT JOIN spaces s ON s.id = p.destination_space_id
		WHERE p.id = $1 AND p.status = 'published'`, id).
		Scan(&r.ID, &r.DestinationSpaceID, &r.DestinationName,
			&r.ProgramLevel, &r.FieldOfStudy, &r.Title, &r.Body, &r.WordCount,
			&r.Status, &r.AIGenerated, &r.IsJapabotIndexed, &r.CreatedAt)
	return r, err
}

// CreateDraft inserts a queued draft owned by the user.
func (s *Store) CreateDraft(ctx context.Context, d DraftRow) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO sop_drafts
		  (user_id, destination_space_id, purpose, program, institution, title, inputs, ai_prompt_hash, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'queued')
		RETURNING id`,
		d.UserID, d.DestinationSpaceID, d.Purpose, d.Program, d.Institution, d.Title, d.Inputs, d.AIPromptHash).Scan(&id)
	return id, err
}

// GetDraft returns a draft by id (any status). Callers must check
// ownership.
func (s *Store) GetDraft(ctx context.Context, id pgtype.UUID) (DraftRow, error) {
	var d DraftRow
	err := s.pool.Reader.QueryRow(ctx, `
		SELECT id, user_id, destination_space_id, purpose, program, institution,
		       title, inputs, generated_text, status, ai_model, ai_prompt_hash, version, created_at, updated_at
		FROM sop_drafts
		WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&d.ID, &d.UserID, &d.DestinationSpaceID, &d.Purpose, &d.Program, &d.Institution,
			&d.Title, &d.Inputs, &d.GeneratedText, &d.Status, &d.AIModel, &d.AIPromptHash, &d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

// ListDrafts returns a user's drafts, newest first.
func (s *Store) ListDrafts(ctx context.Context, userID pgtype.UUID, page, limit int32) ([]DraftRow, int64, error) {
	var total int64
	if err := s.pool.Reader.QueryRow(ctx,
		`SELECT count(*) FROM sop_drafts WHERE user_id = $1 AND deleted_at IS NULL`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Reader.Query(ctx, `
		SELECT id, user_id, destination_space_id, purpose, program, institution,
		       title, inputs, generated_text, status, ai_model, version, created_at, updated_at
		FROM sop_drafts
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, userID, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []DraftRow
	for rows.Next() {
		var d DraftRow
		if err := rows.Scan(&d.ID, &d.UserID, &d.DestinationSpaceID, &d.Purpose, &d.Program, &d.Institution,
			&d.Title, &d.Inputs, &d.GeneratedText, &d.Status, &d.AIModel, &d.Version, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

// UpdateDraft edits a user draft (title, program, institution, and manual
// text edits — bumps version).
func (s *Store) UpdateDraft(ctx context.Context, id, userID pgtype.UUID, title, program, institution, generatedText *string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_drafts SET
		  title = COALESCE($3, title),
		  program = COALESCE($4, program),
		  institution = COALESCE($5, institution),
		  generated_text = COALESCE($6, generated_text),
		  version = version + 1,
		  updated_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID, title, program, institution, generatedText)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// SoftDeleteDraft marks a draft deleted (Rule 11).
func (s *Store) SoftDeleteDraft(ctx context.Context, id, userID pgtype.UUID) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_drafts SET deleted_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// MarkDraftGenerated stores the generated output (sop:generate worker).
func (s *Store) MarkDraftGenerated(ctx context.Context, id pgtype.UUID, text string, model *string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_drafts
		SET generated_text = $2, ai_model = $3, status = 'generated', updated_at = now()
		WHERE id = $1 AND status = 'queued'`, id, text, model)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// MarkDraftFailed flips a queued draft to failed (sop:generate worker).
func (s *Store) MarkDraftFailed(ctx context.Context, id pgtype.UUID) error {
	_, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_drafts SET status = 'failed', updated_at = now()
		WHERE id = $1 AND status = 'queued'`, id)
	return err
}

// CountDraftsThisMonth is the quota fallback when Valkey is unavailable —
// counts drafts created in the current UTC month.
func (s *Store) CountDraftsThisMonth(ctx context.Context, userID pgtype.UUID) (int64, error) {
	var n int64
	err := s.pool.Reader.QueryRow(ctx, `
		SELECT count(*) FROM sop_drafts
		WHERE user_id = $1 AND deleted_at IS NULL
		  AND created_at >= date_trunc('month', now())`, userID).Scan(&n)
	return n, err
}

// AdminListTemplates returns templates in any status (admin).
func (s *Store) AdminListTemplates(ctx context.Context, page, limit int32) ([]TemplateRow, int64, error) {
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, `SELECT count(*) FROM sop_templates`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT id, destination_space_id, purpose, title, structure, status, is_active,
		       ai_generated, created_at, updated_at
		FROM sop_templates
		ORDER BY updated_at DESC
		LIMIT $1 OFFSET $2`, limit, (page-1)*limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []TemplateRow
	for rows.Next() {
		var t TemplateRow
		if err := rows.Scan(&t.ID, &t.DestinationSpaceID, &t.Purpose, &t.Title,
			&t.Structure, &t.Status, &t.IsActive, &t.AIGenerated,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// CreateTemplate inserts a template (admin form or tool:generate_draft
// Content Agent output).
func (s *Store) CreateTemplate(ctx context.Context, t TemplateRow) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO sop_templates
		  (destination_space_id, purpose, title, structure, status, ai_generated)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id`,
		t.DestinationSpaceID, t.Purpose, t.Title, t.Structure, t.Status, t.AIGenerated).Scan(&id)
	return id, err
}

// ApproveSample publishes a sample (HITL).
func (s *Store) ApproveSample(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_samples
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

// ApproveTemplate publishes a template (HITL approve path for Content
// Agent drafts).
func (s *Store) ApproveTemplate(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_templates
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

// RejectTemplate marks a template draft rejected (HITL).
func (s *Store) RejectTemplate(ctx context.Context, id pgtype.UUID) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_templates SET status = 'rejected', updated_at = now()
		WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// RejectSample marks a pending/draft sample rejected (HITL).
func (s *Store) RejectSample(ctx context.Context, id, reviewerID pgtype.UUID, notes string) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_samples
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

// UpdateTemplate applies a partial update (admin).
func (s *Store) UpdateTemplate(ctx context.Context, id pgtype.UUID, t TemplateRow) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_templates SET
		  destination_space_id = COALESCE($2, destination_space_id),
		  purpose = COALESCE(NULLIF($3,''), purpose),
		  title = COALESCE(NULLIF($4,''), title),
		  structure = COALESCE($5, structure),
		  is_active = COALESCE($6, is_active),
		  updated_at = now()
		WHERE id = $1`,
		id, t.DestinationSpaceID, t.Purpose, t.Title,
		nullBytes(t.Structure), t.IsActive)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteTemplate removes a non-published template.
func (s *Store) DeleteTemplate(ctx context.Context, id pgtype.UUID) error {
	res, err := s.pool.Writer.Exec(ctx, `
		DELETE FROM sop_templates
		WHERE id = $1 AND status IN ('draft','ai_draft','pending','rejected')`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// AdminListSamples returns all samples regardless of status (admin).
func (s *Store) AdminListSamples(ctx context.Context, destSpaceID pgtype.UUID, status string, page, limit int32) ([]SampleRow, int64, error) {
	where := ` WHERE true`
	args := []any{}
	if destSpaceID.Valid {
		args = append(args, destSpaceID)
		where += ` AND s.destination_space_id = $` + itoa(len(args))
	}
	if status != "" {
		args = append(args, status)
		where += ` AND s.status = $` + itoa(len(args))
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx,
		`SELECT count(*) FROM sop_samples s`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, (page-1)*limit)
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT s.id, s.destination_space_id, sp.name, s.program_level,
		       s.field_of_study, s.title, s.body, s.word_count, s.status,
		       s.ai_generated, s.is_japabot_indexed, s.created_at
		FROM sop_samples s
		LEFT JOIN spaces sp ON sp.id = s.destination_space_id`+
		where+` ORDER BY s.updated_at DESC LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []SampleRow
	for rows.Next() {
		var r SampleRow
		if err := rows.Scan(&r.ID, &r.DestinationSpaceID, &r.DestinationName,
			&r.ProgramLevel, &r.FieldOfStudy, &r.Title, &r.Body,
			&r.WordCount, &r.Status, &r.AIGenerated, &r.IsJapabotIndexed,
			&r.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// CreateSample inserts a sample (admin).
func (s *Store) CreateSample(ctx context.Context, r SampleRow) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO sop_samples
		  (destination_space_id, program_level, field_of_study, title, body,
		   word_count, status, ai_generated)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		r.DestinationSpaceID, r.ProgramLevel, r.FieldOfStudy, r.Title,
		r.Body, r.WordCount, r.Status, r.AIGenerated).Scan(&id)
	return id, err
}

// UpdateSample applies a partial update (admin).
func (s *Store) UpdateSample(ctx context.Context, id pgtype.UUID, r SampleRow) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_samples SET
		  destination_space_id = COALESCE($2, destination_space_id),
		  program_level = COALESCE(NULLIF($3,''), program_level),
		  field_of_study = COALESCE($4, field_of_study),
		  title = COALESCE(NULLIF($5,''), title),
		  body = COALESCE(NULLIF($6,''), body),
		  word_count = COALESCE($7, word_count),
		  updated_at = now()
		WHERE id = $1`,
		id, r.DestinationSpaceID, r.ProgramLevel, r.FieldOfStudy,
		r.Title, r.Body, r.WordCount)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// DeleteSample removes a non-published sample.
func (s *Store) DeleteSample(ctx context.Context, id pgtype.UUID) error {
	res, err := s.pool.Writer.Exec(ctx, `
		DELETE FROM sop_samples
		WHERE id = $1 AND status IN ('draft','ai_draft','pending','rejected')`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// nullBytes returns nil for empty byte slices (COALESCE pattern).
func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// ---------------------------------------------------------------------
// Fields (sop_fields) — curated field-of-study dictionary
// ---------------------------------------------------------------------

// FieldRow is the internal sop_fields representation.
type FieldRow struct {
	ID        pgtype.UUID        `json:"id"`
	Name      string             `json:"name"`
	IsActive  bool               `json:"is_active"`
	SortOrder int32              `json:"sort_order"`
	CreatedAt pgtype.Timestamptz `json:"created_at"`
	UpdatedAt pgtype.Timestamptz `json:"updated_at"`
}

// ListActiveFields returns active field options for the public select,
// ordered by sort_order then name.
func (s *Store) ListActiveFields(ctx context.Context) ([]FieldRow, error) {
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT id, name, is_active, sort_order, created_at, updated_at
		FROM sop_fields
		WHERE is_active
		ORDER BY sort_order, lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []FieldRow
	for rows.Next() {
		var r FieldRow
		if err := rows.Scan(&r.ID, &r.Name, &r.IsActive, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AdminListFields returns all field rows (any is_active) matching an
// optional case-insensitive name search, with total count for pagination.
func (s *Store) AdminListFields(ctx context.Context, q string, page, limit int32) ([]FieldRow, int64, error) {
	where := ` WHERE true`
	args := []any{}
	if q != "" {
		args = append(args, q)
		where += ` AND name ILIKE '%' || $` + itoa(len(args)) + ` || '%'`
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx,
		`SELECT count(*) FROM sop_fields`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, (page-1)*limit)
	rows, err := s.pool.Reader.Query(ctx, `
		SELECT id, name, is_active, sort_order, created_at, updated_at
		FROM sop_fields`+
		where+` ORDER BY sort_order, lower(name) LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []FieldRow
	for rows.Next() {
		var r FieldRow
		if err := rows.Scan(&r.ID, &r.Name, &r.IsActive, &r.SortOrder, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

// CreateField inserts a field row and returns its id.
func (s *Store) CreateField(ctx context.Context, r FieldRow) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
		INSERT INTO sop_fields (name, is_active, sort_order)
		VALUES ($1, $2, $3)
		RETURNING id`,
		r.Name, r.IsActive, r.SortOrder).Scan(&id)
	return id, err
}

// UpdateField applies a partial update via COALESCE (admin).
func (s *Store) UpdateField(ctx context.Context, id pgtype.UUID, name *string, isActive *bool, sortOrder *int32) error {
	res, err := s.pool.Writer.Exec(ctx, `
		UPDATE sop_fields SET
		  name = COALESCE(NULLIF($2,''), name),
		  is_active = COALESCE($3, is_active),
		  sort_order = COALESCE($4, sort_order),
		  updated_at = now()
		WHERE id = $1`,
		id, name, isActive, sortOrder)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// EnsureField inserts a field row when absent — auto-curates the
// dictionary when a sample carries a field_of_study value not yet in the
// managed list. ON CONFLICT DO NOTHING makes repeated writes idempotent.
func (s *Store) EnsureField(ctx context.Context, name string) error {
	_, err := s.pool.Writer.Exec(ctx, `
		INSERT INTO sop_fields (name)
		VALUES ($1)
		ON CONFLICT DO NOTHING`, name)
	return err
}

// DeleteField removes a field row. Samples keep their stored
// field_of_study text — deleting a field only removes the option.
func (s *Store) DeleteField(ctx context.Context, id pgtype.UUID) error {
	res, err := s.pool.Writer.Exec(ctx, `DELETE FROM sop_fields WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
