// Package languagetesthub implements the Language Test Preparation Hub
// (§99, ADR-107).
// // The package manages four tables created by migration 000339:
// language_tests (the canonical list of tests: IELTS, OET, TOEFL, PTE,
// CELPIP, Goethe, DELF, other), language_test_requirements (per-tenant
// minimum-score requirements keyed by destination space + visa category +
// test), language_test_resources (curated prep resources per test), and
// language_score_stories (user-submitted score stories). It uses direct
// pgx queries because these are new tables without sqlc-generated queries
// yet. Tenancy uses tenants(id); geo entities use spaces(id) per ADR-053.
// // Ref: japaconnect-engineering/sections/99-language-test-hub.md
// //	japaconnect-engineering/adr/ADR-107-language-test-hub.md
package languagetesthub

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Requirement statuses (§99).
const (
	StatusDraft     = "draft"
	StatusAIDraft   = "ai_draft"
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

// Resource types (§99).
const (
	ResourceTypeFree      = "free"
	ResourceTypePaid      = "paid"
	ResourceTypeOfficial  = "official"
	ResourceTypeCommunity = "community"
)

// Store is the data-access layer for the Language Test Preparation Hub
// tables. It uses direct pgx queries because the tables are new and have
// no sqlc-generated queries yet. No business logic here.
type Store struct {
	pool *db.DB
}

// NewStore creates a language test hub store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// TestRow holds a single language_tests row.
type TestRow struct {
	ID             pgtype.UUID
	Slug           string
	Name           string
	OfficialURL    *string
	ValidityMonths *int32
}

// RequirementRow holds a joined language_test_requirements +
// language_tests + spaces row, returned by list and admin-list queries.
type RequirementRow struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID
	DestSpaceName      *string
	DestSpaceCode      *string
	VisaCategory       *string
	TestID             pgtype.UUID
	TestSlug           string
	TestName           string
	MinimumScore       string
	ScoreBreakdown     []byte // jsonb
	Notes              *string
	SourceURL          *string
	Status             string
	IsActive           bool
	AIGenerated        bool
	AIModel            *string
	AIPromptHash       *string
	HITLReviewedBy     pgtype.UUID
	HITLReviewedAt     pgtype.Timestamptz
	HITLReviewNotes    *string
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

// ResourceRow holds a single language_test_resources row.
type ResourceRow struct {
	ID              pgtype.UUID
	TenantID        pgtype.UUID
	TestID          pgtype.UUID
	Title           string
	ResourceURL     string
	ResourceType    *string
	CommunityRating *string // numeric(3,2) scanned as text
	RatingCount     int32
	CreatedAt       pgtype.Timestamptz
}

// ScoreStoryRow holds a joined language_score_stories + users row.
type ScoreStoryRow struct {
	ID                 pgtype.UUID
	TenantID           pgtype.UUID
	UserID             pgtype.UUID
	TestID             pgtype.UUID
	StartingLevel      *string
	FinalScore         string
	DestinationSpaceID pgtype.UUID
	DestSpaceName      *string
	PrepDurationWeeks  *int32
	ResourcesUsed      []string
	Advice             *string
	Status             string
	CreatedAt          pgtype.Timestamptz
	AuthorName         *string
}

// ListTests returns all language_tests rows ordered by id.
func (s *Store) ListTests(ctx context.Context) ([]TestRow, error) {
	const q = `
SELECT id, slug, name, official_url, validity_months
FROM language_tests
ORDER BY id
`

	rows, err := s.pool.Reader.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: list tests: %w", err)
	}

	defer rows.Close()

	var out []TestRow

	for rows.Next() {
		var r TestRow
		if err := rows.Scan(&r.ID, &r.Slug, &r.Name, &r.OfficialURL, &r.ValidityMonths); err != nil {
			return nil, fmt.Errorf("languagetesthub: scan test row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("languagetesthub: iterate test rows: %w", err)
	}

	return out, nil
}

// GetTestBySlug returns a single language_tests row by its slug.
func (s *Store) GetTestBySlug(ctx context.Context, slug string) (TestRow, error) {
	const q = `
SELECT id, slug, name, official_url, validity_months
FROM language_tests
WHERE slug = $1
LIMIT 1
`

	var r TestRow

	err := s.pool.Reader.QueryRow(ctx, q, slug).Scan(&r.ID, &r.Slug, &r.Name, &r.OfficialURL, &r.ValidityMonths)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TestRow{}, pgx.ErrNoRows
		}

		return TestRow{}, fmt.Errorf("languagetesthub: get test by slug: %w", err)
	}

	return r, nil
}

// GetTestByID returns a single language_tests row by its id.
func (s *Store) GetTestByID(ctx context.Context, id pgtype.UUID) (TestRow, error) {
	const q = `
SELECT id, slug, name, official_url, validity_months
FROM language_tests
WHERE id = $1
LIMIT 1
`

	var r TestRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(&r.ID, &r.Slug, &r.Name, &r.OfficialURL, &r.ValidityMonths)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TestRow{}, pgx.ErrNoRows
		}

		return TestRow{}, fmt.Errorf("languagetesthub: get test by id: %w", err)
	}

	return r, nil
}

// SearchTests searches language_tests by name or slug.
func (s *Store) SearchTests(ctx context.Context, query string, limit int32) ([]TestRow, error) {
	const q = `
SELECT id, slug, name, official_url, validity_months
FROM language_tests
WHERE (name ILIKE $1 OR slug ILIKE $1)
ORDER BY name
LIMIT $2
`

	rows, err := s.pool.Reader.Query(ctx, q, "%"+query+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: search tests: %w", err)
	}

	defer rows.Close()

	var out []TestRow

	for rows.Next() {
		var r TestRow
		if err := rows.Scan(&r.ID, &r.Slug, &r.Name, &r.OfficialURL, &r.ValidityMonths); err != nil {
			return nil, fmt.Errorf("languagetesthub: scan test row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("languagetesthub: iterate test rows: %w", err)
	}

	return out, nil
}

// ListRequirementsFilter holds the filter parameters for listing
// requirements. DestinationSpaceID and VisaCategory are optional filters
// used by the public requirements endpoint.
type ListRequirementsFilter struct {
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID // optional
	VisaCategory       string      // optional
	TestID             pgtype.UUID // optional (zero = no filter)
	Status             string      // default: published
	IsActive           *bool       // default: true
	Limit              int32
	Offset             int32
}

// ListRequirements returns published, active requirements for the public
// requirements endpoint, optionally filtered by destination space and
// visa category.
func (s *Store) ListRequirements(ctx context.Context, f ListRequirementsFilter) ([]RequirementRow, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("r.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	} else {
		conditions = append(conditions, "r.status = 'published'")
	}

	active := true
	if f.IsActive != nil {
		active = *f.IsActive
	}

	conditions = append(conditions, fmt.Sprintf("r.is_active = $%d", argIdx))
	args = append(args, active)
	argIdx++

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("r.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.VisaCategory != "" {
		conditions = append(conditions, fmt.Sprintf("r.visa_category = $%d", argIdx))
		args = append(args, f.VisaCategory)
		argIdx++
	}

	if f.TestID.Valid {
		conditions = append(conditions, fmt.Sprintf("r.test_id = $%d", argIdx))
		args = append(args, f.TestID)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

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

	q := fmt.Sprintf(`
SELECT r.id, r.destination_space_id, ds.name AS dest_space_name,
       ds.geo_code AS dest_space_code, r.visa_category, r.test_id,
       t.slug AS test_slug, t.name AS test_name, r.minimum_score,
       r.score_breakdown, r.notes, r.source_url, r.status, r.is_active,
       r.ai_generated, r.ai_model, r.ai_prompt_hash, r.hitl_reviewed_by,
       r.hitl_reviewed_at, r.hitl_review_notes, r.created_at, r.updated_at
FROM language_test_requirements r
JOIN language_tests t ON t.id = r.test_id
LEFT JOIN spaces ds ON ds.id = r.destination_space_id
WHERE %s
ORDER BY r.destination_space_id, r.visa_category NULLS FIRST, r.test_id
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: list requirements: %w", err)
	}
	defer rows.Close()

	var out []RequirementRow

	for rows.Next() {
		var r RequirementRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID, &r.DestSpaceName,
			&r.DestSpaceCode, &r.VisaCategory, &r.TestID,
			&r.TestSlug, &r.TestName, &r.MinimumScore,
			&r.ScoreBreakdown, &r.Notes, &r.SourceURL, &r.Status, &r.IsActive,
			&r.AIGenerated, &r.AIModel, &r.AIPromptHash, &r.HITLReviewedBy,
			&r.HITLReviewedAt, &r.HITLReviewNotes, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("languagetesthub: scan requirement row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("languagetesthub: iterate requirement rows: %w", err)
	}

	return out, nil
}

// AdminListRequirementsFilter — AdminListRequirements returns requirements (any status) for the admin
// moderation queue. Supports optional filters by status and destination.
type AdminListRequirementsFilter struct {
	TenantID           pgtype.UUID
	Status             string // optional
	DestinationSpaceID pgtype.UUID
	Q                  string // optional search on test name / notes
	Limit              int32
	Offset             int32
}

// AdminListRequirements returns all requirements (any status) for the
// admin moderation queue.
func (s *Store) AdminListRequirements(ctx context.Context, f AdminListRequirementsFilter) ([]RequirementRow, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if f.Status != "" {
		conditions = append(conditions, fmt.Sprintf("r.status = $%d", argIdx))
		args = append(args, f.Status)
		argIdx++
	}

	if f.DestinationSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("r.destination_space_id = $%d", argIdx))
		args = append(args, f.DestinationSpaceID)
		argIdx++
	}

	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("(t.name ILIKE $%d OR r.notes ILIKE $%d)", argIdx, argIdx))
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
SELECT r.id, r.destination_space_id, ds.name AS dest_space_name,
       ds.geo_code AS dest_space_code, r.visa_category, r.test_id,
       t.slug AS test_slug, t.name AS test_name, r.minimum_score,
       r.score_breakdown, r.notes, r.source_url, r.status, r.is_active,
       r.ai_generated, r.ai_model, r.ai_prompt_hash, r.hitl_reviewed_by,
       r.hitl_reviewed_at, r.hitl_review_notes, r.created_at, r.updated_at
FROM language_test_requirements r
JOIN language_tests t ON t.id = r.test_id
LEFT JOIN spaces ds ON ds.id = r.destination_space_id
WHERE %s
ORDER BY r.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("languagetesthub: admin list requirements: %w", err)
	}
	defer rows.Close()

	var out []RequirementRow

	for rows.Next() {
		var r RequirementRow
		if err := rows.Scan(
			&r.ID, &r.DestinationSpaceID, &r.DestSpaceName,
			&r.DestSpaceCode, &r.VisaCategory, &r.TestID,
			&r.TestSlug, &r.TestName, &r.MinimumScore,
			&r.ScoreBreakdown, &r.Notes, &r.SourceURL, &r.Status, &r.IsActive,
			&r.AIGenerated, &r.AIModel, &r.AIPromptHash, &r.HITLReviewedBy,
			&r.HITLReviewedAt, &r.HITLReviewNotes, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("languagetesthub: admin scan requirement row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("languagetesthub: admin iterate requirement rows: %w", err)
	}

	countQuery := fmt.Sprintf(`
SELECT COUNT(*) FROM language_test_requirements r
JOIN language_tests t ON t.id = r.test_id
WHERE %s
`, whereClause)
	countArgs := args[:len(args)-2]

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("languagetesthub: admin count requirements: %w", err)
	}

	return out, total, nil
}

// CreateRequirementInput holds the fields for inserting a new requirement.
type CreateRequirementInput struct {
	TenantID           pgtype.UUID
	DestinationSpaceID pgtype.UUID
	VisaCategory       *string
	TestID             pgtype.UUID
	MinimumScore       string
	ScoreBreakdown     []byte // jsonb
	Notes              *string
	SourceURL          *string
	Status             string
	IsActive           bool
	AIGenerated        bool
	AIModel            *string
	AIPromptHash       *string
}

// CreateRequirement inserts a new language_test_requirements row and
// returns the inserted row.
func (s *Store) CreateRequirement(ctx context.Context, in CreateRequirementInput) (RequirementRow, error) {
	status := in.Status
	if status == "" {
		status = StatusPublished
	}

	scoreBreakdown := in.ScoreBreakdown
	if scoreBreakdown == nil {
		scoreBreakdown = []byte("null")
	}

	const q = `
INSERT INTO language_test_requirements (
    destination_space_id, visa_category, test_id, minimum_score,
    score_breakdown, notes, source_url, status, is_active,
    ai_generated, ai_model, ai_prompt_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING id, destination_space_id, visa_category, test_id,
          minimum_score, score_breakdown, notes, source_url, status,
          is_active, ai_generated, ai_model, ai_prompt_hash,
          hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes,
          created_at, updated_at
`

	var r RequirementRow

	err := s.pool.Writer.QueryRow(
		ctx, q,
		in.TenantID, in.DestinationSpaceID, in.VisaCategory, in.TestID,
		in.MinimumScore, scoreBreakdown, in.Notes, in.SourceURL, status,
		in.IsActive, in.AIGenerated, in.AIModel, in.AIPromptHash,
	).Scan(
		&r.ID, &r.DestinationSpaceID, &r.VisaCategory, &r.TestID,
		&r.MinimumScore, &r.ScoreBreakdown, &r.Notes, &r.SourceURL, &r.Status,
		&r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return RequirementRow{}, fmt.Errorf("languagetesthub: create requirement: %w", err)
	}
	// Populate joined fields (test slug/name, space name) via a follow-up
	// read so the returned DTO is complete.
	if err := s.populateRequirementJoins(ctx, &r); err != nil {
		return RequirementRow{}, err
	}

	return r, nil
}

// populateRequirementJoins fills in the joined test/space columns on a
// RequirementRow using the row's test_id and destination_space_id.
func (s *Store) populateRequirementJoins(ctx context.Context, r *RequirementRow) error {
	const q = `
SELECT t.slug, t.name, ds.name, ds.geo_code
FROM language_tests t
LEFT JOIN spaces ds ON ds.id = $2
WHERE t.id = $1
`

	if err := s.pool.Reader.QueryRow(ctx, q, r.TestID, r.DestinationSpaceID).Scan(
		&r.TestSlug, &r.TestName, &r.DestSpaceName, &r.DestSpaceCode,
	); err != nil {
		return fmt.Errorf("languagetesthub: populate requirement joins: %w", err)
	}

	return nil
}

// UpdateRequirementInput holds the mutable fields for a partial update
// of a language_test_requirements row. Nil/zero pointer fields are ignored.
type UpdateRequirementInput struct {
	DestinationSpaceID pgtype.UUID
	VisaCategory       *string
	TestID             pgtype.UUID
	MinimumScore       *string
	ScoreBreakdown     []byte // jsonb
	Notes              *string
	SourceURL          *string
	Status             *string
	IsActive           *bool
	AIGenerated        *bool
	AIModel            *string
	AIPromptHash       *string
}

// GetRequirementByID returns a single language_test_requirements row
// by id, scoped to a tenant, with test and destination-space joins.
func (s *Store) GetRequirementByID(ctx context.Context, id pgtype.UUID) (RequirementRow, error) {
	const q = `
SELECT r.id, r.destination_space_id, ds.name AS dest_space_name,
       ds.geo_code AS dest_space_code, r.visa_category, r.test_id,
       t.slug AS test_slug, t.name AS test_name, r.minimum_score,
       r.score_breakdown, r.notes, r.source_url, r.status, r.is_active,
       r.ai_generated, r.ai_model, r.ai_prompt_hash, r.hitl_reviewed_by,
       r.hitl_reviewed_at, r.hitl_review_notes, r.created_at, r.updated_at
FROM language_test_requirements r
JOIN language_tests t ON t.id = r.test_id
LEFT JOIN spaces ds ON ds.id = r.destination_space_id
WHERE r.id = $1
LIMIT 1
`

	var r RequirementRow

	err := s.pool.Reader.QueryRow(ctx, q, id).Scan(
		&r.ID, &r.DestinationSpaceID, &r.DestSpaceName,
		&r.DestSpaceCode, &r.VisaCategory, &r.TestID,
		&r.TestSlug, &r.TestName, &r.MinimumScore,
		&r.ScoreBreakdown, &r.Notes, &r.SourceURL, &r.Status, &r.IsActive,
		&r.AIGenerated, &r.AIModel, &r.AIPromptHash, &r.HITLReviewedBy,
		&r.HITLReviewedAt, &r.HITLReviewNotes, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RequirementRow{}, pgx.ErrNoRows
		}

		return RequirementRow{}, fmt.Errorf("languagetesthub: get requirement by id: %w", err)
	}

	return r, nil
}

// UpdateRequirement applies a tenant-scoped partial update to a
// language_test_requirements row. Only non-nil/valid fields in the input
// are changed. It returns the updated row.
func (s *Store) UpdateRequirement(ctx context.Context, id, tenantID pgtype.UUID, in UpdateRequirementInput) (RequirementRow, error) {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if in.DestinationSpaceID.Valid {
		sets = append(sets, fmt.Sprintf("destination_space_id = $%d", argIdx))
		args = append(args, in.DestinationSpaceID)
		argIdx++
	}

	if in.VisaCategory != nil {
		sets = append(sets, fmt.Sprintf("visa_category = $%d", argIdx))
		args = append(args, *in.VisaCategory)
		argIdx++
	}

	if in.TestID.Valid {
		sets = append(sets, fmt.Sprintf("test_id = $%d", argIdx))
		args = append(args, in.TestID)
		argIdx++
	}

	if in.MinimumScore != nil {
		sets = append(sets, fmt.Sprintf("minimum_score = $%d", argIdx))
		args = append(args, *in.MinimumScore)
		argIdx++
	}

	if in.ScoreBreakdown != nil {
		sets = append(sets, fmt.Sprintf("score_breakdown = $%d", argIdx))
		args = append(args, in.ScoreBreakdown)
		argIdx++
	}

	if in.Notes != nil {
		sets = append(sets, fmt.Sprintf("notes = $%d", argIdx))
		args = append(args, *in.Notes)
		argIdx++
	}

	if in.SourceURL != nil {
		sets = append(sets, fmt.Sprintf("source_url = $%d", argIdx))
		args = append(args, *in.SourceURL)
		argIdx++
	}

	if in.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *in.Status)
		argIdx++
	}

	if in.IsActive != nil {
		sets = append(sets, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *in.IsActive)
		argIdx++
	}

	if in.AIGenerated != nil {
		sets = append(sets, fmt.Sprintf("ai_generated = $%d", argIdx))
		args = append(args, *in.AIGenerated)
		argIdx++
	}

	if in.AIModel != nil {
		sets = append(sets, fmt.Sprintf("ai_model = $%d", argIdx))
		args = append(args, *in.AIModel)
		argIdx++
	}

	if in.AIPromptHash != nil {
		sets = append(sets, fmt.Sprintf("ai_prompt_hash = $%d", argIdx))
		args = append(args, *in.AIPromptHash)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetRequirementByID(ctx, id)
	}

	sets = append(sets, "updated_at = now()")
	args = append(args, id)

	q := fmt.Sprintf(`
UPDATE language_test_requirements
SET %s
WHERE id = $%d
RETURNING id, destination_space_id, visa_category, test_id,
          minimum_score, score_breakdown, notes, source_url, status,
          is_active, ai_generated, ai_model, ai_prompt_hash,
          hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes,
          created_at, updated_at
`, strings.Join(sets, ", "), argIdx)

	var r RequirementRow

	err := s.pool.Writer.QueryRow(ctx, q, args...).Scan(
		&r.ID, &r.DestinationSpaceID, &r.VisaCategory, &r.TestID,
		&r.MinimumScore, &r.ScoreBreakdown, &r.Notes, &r.SourceURL, &r.Status,
		&r.IsActive, &r.AIGenerated, &r.AIModel, &r.AIPromptHash,
		&r.HITLReviewedBy, &r.HITLReviewedAt, &r.HITLReviewNotes,
		&r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RequirementRow{}, pgx.ErrNoRows
		}

		return RequirementRow{}, fmt.Errorf("languagetesthub: update requirement: %w", err)
	}

	if err := s.populateRequirementJoins(ctx, &r); err != nil {
		return RequirementRow{}, err
	}

	return r, nil
}

// DeleteRequirement deletes a language_test_requirements row by id,
// scoped to a tenant.
func (s *Store) DeleteRequirement(ctx context.Context, id, tenantID pgtype.UUID) error {
	const q = `DELETE FROM language_test_requirements WHERE id = $1`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID)
	if err != nil {
		return fmt.Errorf("languagetesthub: delete requirement: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ApproveRequirement marks a requirement as reviewed and published by an
// admin (HITL approve). Sets hitl_reviewed_by, hitl_reviewed_at=now(),
// status='published', and optional review notes.
func (s *Store) ApproveRequirement(ctx context.Context, id, tenantID, reviewerID pgtype.UUID, reviewNotes *string) error {
	const q = `
UPDATE language_test_requirements
SET hitl_reviewed_by = $2,
    hitl_reviewed_at = now(),
    hitl_review_notes = $3,
    status = 'published',
    updated_at = now()
WHERE id = $1
`

	cmd, err := s.pool.Writer.Exec(ctx, q, id, tenantID, reviewerID, reviewNotes)
	if err != nil {
		return fmt.Errorf("languagetesthub: approve requirement: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// ListResources returns published resources for a test, ordered by
// community rating descending.
func (s *Store) ListResources(ctx context.Context, tenantID, testID pgtype.UUID, limit, offset int32) ([]ResourceRow, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	if offset < 0 {
		offset = 0
	}

	const q = `
SELECT id, test_id, title, resource_url, resource_type,
       community_rating::text, rating_count, created_at
FROM language_test_resources
WHERE test_id = $1
ORDER BY community_rating DESC NULLS LAST, created_at DESC
LIMIT $2 OFFSET $3
`

	rows, err := s.pool.Reader.Query(ctx, q, testID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: list resources: %w", err)
	}

	defer rows.Close()

	var out []ResourceRow

	for rows.Next() {
		var r ResourceRow
		if err := rows.Scan(
			&r.ID, &r.TestID, &r.Title, &r.ResourceURL,
			&r.ResourceType, &r.CommunityRating, &r.RatingCount, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("languagetesthub: scan resource row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("languagetesthub: iterate resource rows: %w", err)
	}

	return out, nil
}

// ListStories returns published score stories for a test, optionally
// filtered by destination space, ordered by created_at DESC.
func (s *Store) ListStories(ctx context.Context, tenantID, testID, destSpaceID pgtype.UUID, limit, offset int32) ([]ScoreStoryRow, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	if offset < 0 {
		offset = 0
	}

	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("s.test_id = $%d", argIdx))
	args = append(args, testID)
	argIdx++

	conditions = append(conditions, fmt.Sprintf("s.status = $%d", argIdx))
	args = append(args, StatusPublished)
	argIdx++

	conditions = append(conditions, "s.deleted_at IS NULL")

	if destSpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("s.destination_space_id = $%d", argIdx))
		args = append(args, destSpaceID)
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	q := fmt.Sprintf(`
SELECT s.id, s.user_id, s.test_id, s.starting_level,
       s.final_score, s.destination_space_id, ds.name AS dest_space_name,
       s.prep_duration_weeks, s.resources_used, s.advice, s.status,
       s.created_at, u.username AS author_name
FROM language_score_stories s
LEFT JOIN spaces ds ON ds.id = s.destination_space_id
LEFT JOIN users u ON u.id = s.user_id
WHERE %s
ORDER BY s.created_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("languagetesthub: list stories: %w", err)
	}
	defer rows.Close()

	var out []ScoreStoryRow

	for rows.Next() {
		var r ScoreStoryRow
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.TestID, &r.StartingLevel,
			&r.FinalScore, &r.DestinationSpaceID, &r.DestSpaceName,
			&r.PrepDurationWeeks, &r.ResourcesUsed, &r.Advice, &r.Status,
			&r.CreatedAt, &r.AuthorName,
		); err != nil {
			return nil, fmt.Errorf("languagetesthub: scan story row: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("languagetesthub: iterate story rows: %w", err)
	}

	return out, nil
}

// CreateStoryInput holds the fields for inserting a new score story.
type CreateStoryInput struct {
	TenantID           pgtype.UUID
	UserID             pgtype.UUID
	TestID             pgtype.UUID
	StartingLevel      *string
	FinalScore         string
	DestinationSpaceID pgtype.UUID
	PrepDurationWeeks  *int32
	ResourcesUsed      []string
	Advice             *string
	Status             string
}

// CreateStory inserts a new language_score_stories row and returns the
// inserted row (with author name joined).
func (s *Store) CreateStory(ctx context.Context, in CreateStoryInput) (ScoreStoryRow, error) {
	status := in.Status
	if status == "" {
		status = StatusPublished
	}

	const q = `
INSERT INTO language_score_stories (
    user_id, test_id, starting_level, final_score,
    destination_space_id, prep_duration_weeks, resources_used, advice, status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id, user_id, test_id, starting_level, final_score,
          destination_space_id, prep_duration_weeks, resources_used, advice,
          status, created_at
`

	var r ScoreStoryRow

	err := s.pool.Writer.QueryRow(
		ctx, q,
		in.TenantID, in.UserID, in.TestID, in.StartingLevel, in.FinalScore,
		in.DestinationSpaceID, in.PrepDurationWeeks, in.ResourcesUsed,
		in.Advice, status,
	).Scan(
		&r.ID, &r.UserID, &r.TestID, &r.StartingLevel,
		&r.FinalScore, &r.DestinationSpaceID, &r.PrepDurationWeeks,
		&r.ResourcesUsed, &r.Advice, &r.Status, &r.CreatedAt,
	)
	if err != nil {
		return ScoreStoryRow{}, fmt.Errorf("languagetesthub: create story: %w", err)
	}
	// Populate author name join.
	const uq = `SELECT username FROM users WHERE id = $1`
	if err := s.pool.Reader.QueryRow(ctx, uq, in.UserID).Scan(&r.AuthorName); err != nil {
		return ScoreStoryRow{}, fmt.Errorf("languagetesthub: create story: fetch author: %w", err)
	}

	return r, nil
}

// CreateLanguageTestInput holds the fields for inserting a new language test.
type CreateLanguageTestInput struct {
	Slug           string  `json:"slug"`
	Name           string  `json:"name"`
	OfficialURL    *string `json:"official_url,omitempty"`
	ValidityMonths *int32  `json:"validity_months,omitempty"`
}

// CreateLanguageTest inserts a new language_tests row and returns it.
func (s *Store) CreateLanguageTest(ctx context.Context, in CreateLanguageTestInput) (TestRow, error) {
	const q = `
INSERT INTO language_tests (slug, name, official_url, validity_months)
VALUES ($1, $2, $3, $4)
RETURNING id, slug, name, official_url, validity_months
`

	var r TestRow

	err := s.pool.Writer.QueryRow(ctx, q, in.Slug, in.Name, in.OfficialURL, in.ValidityMonths).Scan(
		&r.ID, &r.Slug, &r.Name, &r.OfficialURL, &r.ValidityMonths,
	)
	if err != nil {
		return TestRow{}, fmt.Errorf("languagetesthub: create language test: %w", err)
	}

	return r, nil
}

// UpdateLanguageTestInput holds the mutable fields for a language test update.
type UpdateLanguageTestInput struct {
	Slug           *string `json:"slug,omitempty"`
	Name           *string `json:"name,omitempty"`
	OfficialURL    *string `json:"official_url,omitempty"`
	ValidityMonths *int32  `json:"validity_months,omitempty"`
}

// UpdateLanguageTest updates a language_tests row by id.
func (s *Store) UpdateLanguageTest(ctx context.Context, id pgtype.UUID, in UpdateLanguageTestInput) (TestRow, error) {
	var (
		sets   []string
		args   []any
		argIdx = 1
	)

	if in.Slug != nil {
		sets = append(sets, fmt.Sprintf("slug = $%d", argIdx))
		args = append(args, *in.Slug)
		argIdx++
	}

	if in.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *in.Name)
		argIdx++
	}

	if in.OfficialURL != nil {
		sets = append(sets, fmt.Sprintf("official_url = $%d", argIdx))
		args = append(args, *in.OfficialURL)
		argIdx++
	}

	if in.ValidityMonths != nil {
		sets = append(sets, fmt.Sprintf("validity_months = $%d", argIdx))
		args = append(args, *in.ValidityMonths)
		argIdx++
	}

	if len(sets) == 0 {
		return s.GetTestByID(ctx, id)
	}

	sets = append(sets, "updated_at = now()")
	args = append(args, id)

	q := fmt.Sprintf(
		"UPDATE language_tests SET %s WHERE id = $%d RETURNING id, slug, name, official_url, validity_months",
		strings.Join(sets, ", "),
		argIdx,
	)

	var r TestRow

	err := s.pool.Writer.QueryRow(ctx, q, args...).Scan(
		&r.ID, &r.Slug, &r.Name, &r.OfficialURL, &r.ValidityMonths,
	)
	if err != nil {
		return TestRow{}, fmt.Errorf("languagetesthub: update language test: %w", err)
	}

	return r, nil
}

// DeleteLanguageTest deletes a language_tests row by id.
func (s *Store) DeleteLanguageTest(ctx context.Context, id pgtype.UUID) error {
	cmd, err := s.pool.Writer.Exec(ctx, `DELETE FROM language_tests WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("languagetesthub: delete language test: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}
