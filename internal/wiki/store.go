// Package wiki implements the Destination Wiki (§80, ADR-088).
//
// Collaborative, versioned knowledge-base pages attached to each destination
// country/city Space. Wiki pages are living documents continuously improved
// by reputation-gated community members. Every edit creates a pending
// revision that must be approved by a moderator+ before it becomes the
// current content.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
//      japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db"
)

// Store is the data-access layer for wiki_pages and wiki_revisions.
type Store struct {
	pool *db.DB
}

// NewStore creates a wiki store backed by the given database.
func NewStore(database *db.DB) *Store {
	return &Store{pool: database}
}

// WikiPage represents a single wiki page row.
type WikiPage struct {
	ID               pgtype.UUID
	TenantID         pgtype.UUID
	SpaceID          pgtype.UUID
	Slug             string
	Title            string
	Category         string
	IsActive         bool
	IsFeatured       bool
	IsLocked         bool
	CreatedBy        pgtype.UUID
	CreatedAt        pgtype.Timestamptz
	UpdatedAt        pgtype.Timestamptz
	IsJapabotIndexed bool
}

// WikiPageWithSpace includes the space slug/name for the wiki index listing.
type WikiPageWithSpace struct {
	WikiPage
	SpaceSlug string
	SpaceName string
}

// WikiRevision represents a single revision row.
type WikiRevision struct {
	ID                pgtype.UUID
	TenantID          pgtype.UUID
	WikiPageID        pgtype.UUID
	Content           []byte // TipTap JSON
	Summary           *string
	Status            string // pending|approved|rejected
	SubmittedBy       pgtype.UUID
	ReviewedBy        pgtype.UUID
	ReviewedAt        pgtype.Timestamptz
	AiGenerated       bool
	AiModel           *string
	AiPromptHash      *string
	HitlReviewedBy    pgtype.UUID
	HitlReviewedAt    pgtype.Timestamptz
	HitlReviewNotes   *string
	CreatedAt         pgtype.Timestamptz
	SubmittedByName   *string
}

// GetWikiPageBySlug returns a wiki page by space slug + page slug.
func (s *Store) GetWikiPageBySlug(ctx context.Context, tenantID pgtype.UUID, spaceSlug, pageSlug string) (WikiPageWithSpace, error) {
	const q = `
SELECT wp.id, wp.tenant_id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       sp.slug AS space_slug, sp.name AS space_name
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id AND sp.tenant_id = wp.tenant_id
WHERE wp.tenant_id = $1 AND sp.slug = $2 AND wp.slug = $3
  AND wp.is_active = true
LIMIT 1
`
	var p WikiPageWithSpace
	err := s.pool.Reader.QueryRow(ctx, q, tenantID, spaceSlug, pageSlug).Scan(
		&p.ID, &p.TenantID, &p.SpaceID, &p.Slug, &p.Title, &p.Category,
		&p.IsActive, &p.IsFeatured, &p.IsLocked, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt, &p.IsJapabotIndexed,
		&p.SpaceSlug, &p.SpaceName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return WikiPageWithSpace{}, pgx.ErrNoRows
		}
		return WikiPageWithSpace{}, fmt.Errorf("wiki: get page by slug: %w", err)
	}
	return p, nil
}

// ListWikiPagesBySpace returns all active wiki pages for a space.
func (s *Store) ListWikiPagesBySpace(ctx context.Context, tenantID pgtype.UUID, spaceSlug string) ([]WikiPage, error) {
	const q = `
SELECT wp.id, wp.tenant_id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id AND sp.tenant_id = wp.tenant_id
WHERE wp.tenant_id = $1 AND sp.slug = $2 AND wp.is_active = true
ORDER BY wp.category, wp.title
`
	rows, err := s.pool.Reader.Query(ctx, q, tenantID, spaceSlug)
	if err != nil {
		return nil, fmt.Errorf("wiki: list pages by space: %w", err)
	}
	defer rows.Close()

	var out []WikiPage
	for rows.Next() {
		var p WikiPage
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.SpaceID, &p.Slug, &p.Title, &p.Category,
			&p.IsActive, &p.IsFeatured, &p.IsLocked, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt, &p.IsJapabotIndexed,
		); err != nil {
			return nil, fmt.Errorf("wiki: scan page: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("wiki: iterate pages: %w", err)
	}
	return out, nil
}

// GetApprovedRevision returns the latest approved revision for a wiki page.
func (s *Store) GetApprovedRevision(ctx context.Context, pageID, tenantID pgtype.UUID) (WikiRevision, error) {
	const q = `
SELECT wr.id, wr.tenant_id, wr.wiki_page_id, wr.content, wr.summary,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at, u.username AS submitted_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
WHERE wr.wiki_page_id = $1 AND wr.tenant_id = $2 AND wr.status = 'approved'
ORDER BY wr.created_at DESC
LIMIT 1
`
	var r WikiRevision
	err := s.pool.Reader.QueryRow(ctx, q, pageID, tenantID).Scan(
		&r.ID, &r.TenantID, &r.WikiPageID, &r.Content, &r.Summary,
		&r.Status, &r.SubmittedBy, &r.ReviewedBy, &r.ReviewedAt,
		&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
		&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
		&r.CreatedAt, &r.SubmittedByName,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return WikiRevision{}, pgx.ErrNoRows
		}
		return WikiRevision{}, fmt.Errorf("wiki: get approved revision: %w", err)
	}
	return r, nil
}

// ListRevisions returns the revision history for a wiki page (all statuses).
func (s *Store) ListRevisions(ctx context.Context, pageID, tenantID pgtype.UUID, limit, offset int32) ([]WikiRevision, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	const q = `
SELECT wr.id, wr.tenant_id, wr.wiki_page_id, wr.content, wr.summary,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at, u.username AS submitted_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
WHERE wr.wiki_page_id = $1 AND wr.tenant_id = $2
ORDER BY wr.created_at DESC
LIMIT $3 OFFSET $4
`
	rows, err := s.pool.Reader.Query(ctx, q, pageID, tenantID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: list revisions: %w", err)
	}
	defer rows.Close()

	var out []WikiRevision
	for rows.Next() {
		var r WikiRevision
		if err := rows.Scan(
			&r.ID, &r.TenantID, &r.WikiPageID, &r.Content, &r.Summary,
			&r.Status, &r.SubmittedBy, &r.ReviewedBy, &r.ReviewedAt,
			&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
			&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
			&r.CreatedAt, &r.SubmittedByName,
		); err != nil {
			return nil, 0, fmt.Errorf("wiki: scan revision: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("wiki: iterate revisions: %w", err)
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx,
		`SELECT COUNT(*) FROM wiki_revisions WHERE wiki_page_id = $1 AND tenant_id = $2`,
		pageID, tenantID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("wiki: count revisions: %w", err)
	}

	return out, total, nil
}

// SubmitRevision inserts a new pending revision for a wiki page.
func (s *Store) SubmitRevision(ctx context.Context, tenantID, pageID, userID pgtype.UUID, content []byte, summary string, aiGenerated bool, aiModel, aiPromptHash string) (pgtype.UUID, error) {
	var revID pgtype.UUID
	var summaryPtr *string
	if summary != "" {
		summaryPtr = &summary
	}
	var aiModelPtr, aiPromptHashPtr *string
	if aiModel != "" {
		aiModelPtr = &aiModel
	}
	if aiPromptHash != "" {
		aiPromptHashPtr = &aiPromptHash
	}

	err := s.pool.Writer.QueryRow(ctx, `
INSERT INTO wiki_revisions (tenant_id, wiki_page_id, content, summary, status,
    submitted_by, ai_generated, ai_model, ai_prompt_hash)
VALUES ($1, $2, $3, $4, 'pending', $5, $6, $7, $8)
RETURNING id
`, tenantID, pageID, content, summaryPtr, userID, aiGenerated, aiModelPtr, aiPromptHashPtr).Scan(&revID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("wiki: submit revision: %w", err)
	}
	return revID, nil
}

// ApproveRevision marks a revision as approved and stamps the reviewer.
// Also updates the wiki_page's updated_at.
func (s *Store) ApproveRevision(ctx context.Context, revID, pageID, tenantID, reviewerID pgtype.UUID, reviewNotes string) error {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("wiki: approve: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var notesPtr *string
	if reviewNotes != "" {
		notesPtr = &reviewNotes
	}

	_, err = tx.Exec(ctx, `
UPDATE wiki_revisions
SET status = 'approved', reviewed_by = $3, reviewed_at = now(),
    hitl_reviewed_by = $3, hitl_reviewed_at = now(), hitl_review_notes = $4
WHERE id = $1 AND tenant_id = $2 AND status = 'pending'
`, revID, tenantID, reviewerID, notesPtr)
	if err != nil {
		return fmt.Errorf("wiki: approve: update revision: %w", err)
	}

	_, err = tx.Exec(ctx, `
UPDATE wiki_pages SET updated_at = now() WHERE id = $1 AND tenant_id = $2
`, pageID, tenantID)
	if err != nil {
		return fmt.Errorf("wiki: approve: update page: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("wiki: approve: commit: %w", err)
	}
	return nil
}

// RejectRevision marks a revision as rejected and stamps the reviewer.
func (s *Store) RejectRevision(ctx context.Context, revID, tenantID, reviewerID pgtype.UUID, reviewNotes string) error {
	var notesPtr *string
	if reviewNotes != "" {
		notesPtr = &reviewNotes
	}
	_, err := s.pool.Writer.Exec(ctx, `
UPDATE wiki_revisions
SET status = 'rejected', reviewed_by = $3, reviewed_at = now(),
    hitl_reviewed_by = $3, hitl_reviewed_at = now(), hitl_review_notes = $4
WHERE id = $1 AND tenant_id = $2 AND status = 'pending'
`, revID, tenantID, reviewerID, notesPtr)
	if err != nil {
		return fmt.Errorf("wiki: reject revision: %w", err)
	}
	return nil
}

// UpdatePage updates wiki page admin-managed fields (lock, feature, active).
func (s *Store) UpdatePage(ctx context.Context, pageID, tenantID pgtype.UUID, isLocked, isFeatured, isActive *bool) error {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if isLocked != nil {
		conditions = append(conditions, fmt.Sprintf("is_locked = $%d", argIdx))
		args = append(args, *isLocked)
		argIdx++
	}
	if isFeatured != nil {
		conditions = append(conditions, fmt.Sprintf("is_featured = $%d", argIdx))
		args = append(args, *isFeatured)
		argIdx++
	}
	if isActive != nil {
		conditions = append(conditions, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *isActive)
		argIdx++
	}

	if len(conditions) == 0 {
		return nil // nothing to update
	}

	conditions = append(conditions, "updated_at = now()")
	args = append(args, pageID, tenantID)

	q := fmt.Sprintf(`
UPDATE wiki_pages SET %s WHERE id = $%d AND tenant_id = $%d
`, strings.Join(conditions, ", "), argIdx, argIdx+1)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("wiki: update page: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// CreatePage creates a new wiki page. Used by the admin generate endpoint
// and the space activation seed job.
func (s *Store) CreatePage(ctx context.Context, tenantID, spaceID, createdBy pgtype.UUID, slug, title, category string) (pgtype.UUID, error) {
	var pageID pgtype.UUID
	err := s.pool.Writer.QueryRow(ctx, `
INSERT INTO wiki_pages (tenant_id, space_id, slug, title, category, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (tenant_id, space_id, slug) DO NOTHING
RETURNING id
`, tenantID, spaceID, slug, title, category, createdBy).Scan(&pageID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgtype.UUID{}, ErrPageExists
		}
		return pgtype.UUID{}, fmt.Errorf("wiki: create page: %w", err)
	}
	return pageID, nil
}

// AdminListPagesFilter holds filter parameters for the admin page list.
type AdminListPagesFilter struct {
	TenantID  pgtype.UUID
	SpaceID   pgtype.UUID
	Category  string
	IsLocked  *bool
	Q         string
	Limit     int32
	Offset    int32
}

// AdminListPages returns all wiki pages for admin management (paginated).
func (s *Store) AdminListPages(ctx context.Context, f AdminListPagesFilter) ([]WikiPageWithSpace, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	conditions = append(conditions, fmt.Sprintf("wp.tenant_id = $%d", argIdx))
	args = append(args, f.TenantID)
	argIdx++

	if f.SpaceID.Valid {
		conditions = append(conditions, fmt.Sprintf("wp.space_id = $%d", argIdx))
		args = append(args, f.SpaceID)
		argIdx++
	}
	if f.Category != "" {
		conditions = append(conditions, fmt.Sprintf("wp.category = $%d", argIdx))
		args = append(args, f.Category)
		argIdx++
	}
	if f.IsLocked != nil {
		conditions = append(conditions, fmt.Sprintf("wp.is_locked = $%d", argIdx))
		args = append(args, *f.IsLocked)
		argIdx++
	}
	if f.Q != "" {
		conditions = append(conditions, fmt.Sprintf("wp.title ILIKE $%d", argIdx))
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
SELECT wp.id, wp.tenant_id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       sp.slug AS space_slug, sp.name AS space_name
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id AND sp.tenant_id = wp.tenant_id
WHERE %s
ORDER BY wp.updated_at DESC
LIMIT $%d OFFSET $%d
`, whereClause, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: admin list pages: %w", err)
	}
	defer rows.Close()

	var out []WikiPageWithSpace
	for rows.Next() {
		var p WikiPageWithSpace
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.SpaceID, &p.Slug, &p.Title, &p.Category,
			&p.IsActive, &p.IsFeatured, &p.IsLocked, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt, &p.IsJapabotIndexed,
			&p.SpaceSlug, &p.SpaceName,
		); err != nil {
			return nil, 0, fmt.Errorf("wiki: admin scan page: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("wiki: admin iterate pages: %w", err)
	}

	countQ := fmt.Sprintf(`SELECT COUNT(*) FROM wiki_pages wp WHERE %s`, whereClause)
	countArgs := args[:len(args)-2]
	var total int64
	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("wiki: admin count pages: %w", err)
	}

	return out, total, nil
}

// ErrPageExists is returned when a wiki page with the same slug already
// exists in the same space.
var ErrPageExists = fmt.Errorf("wiki page already exists")
