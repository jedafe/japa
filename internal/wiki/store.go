// Package wiki implements the Destination Wiki (§80, ADR-088).
//
// Collaborative, versioned knowledge-base pages attached to each destination
// country/city Space. Wiki pages are living documents continuously improved
// by reputation-gated community members. Every edit creates a pending
// revision that must be approved by a moderator+ before it becomes the
// current content.
//
// Ref: japaconnect-engineering/sections/80-destination-wiki.md
//
//	japaconnect-engineering/adr/ADR-088-destination-wiki.md
package wiki

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	contentpkg "github.com/jedafe/japa/internal/content"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/rag"
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
//
//nolint:revive // exported: stutter naming is intentional
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
//
//nolint:revive // exported: stutter naming is intentional
type WikiPageWithSpace struct {
	WikiPage
	SpaceSlug string
	SpaceName string
}

// AdminWikiPageWithSpace extends WikiPageWithSpace with the latest revision
// metadata for the admin review queue (B1: contract drift fix). The latest
// revision is the most recently created row for the page, regardless of status.
type AdminWikiPageWithSpace struct {
	WikiPageWithSpace
	LatestRevisionID     *pgtype.UUID
	LatestRevisionStatus *string
	Excerpt              *string
}

// WikiDestination is a destination space (country/state/city) that has at least
// one active wiki page. Region is set on country-level spaces and mirrors the
// spaces.region value (Africa, Americas, Asia, Europe, Oceania).
//
//nolint:revive // exported: stutter naming is intentional
type WikiDestination struct {
	SpaceID   pgtype.UUID
	Slug      string
	Name      string
	GeoCode   *string
	Region    *string
	Level     string
	PageCount int64
}

// WikiRevision represents a single revision row.
//
//nolint:revive // exported: stutter naming is intentional
type WikiRevision struct {
	ID              pgtype.UUID
	TenantID        pgtype.UUID
	WikiPageID      pgtype.UUID
	Content         []byte // TipTap JSON
	Summary         *string
	Status          string // pending|approved|rejected
	SubmittedBy     pgtype.UUID
	ReviewedBy      pgtype.UUID
	ReviewedAt      pgtype.Timestamptz
	AiGenerated     bool
	AiModel         *string
	AiPromptHash    *string
	HitlReviewedBy  pgtype.UUID
	HitlReviewedAt  pgtype.Timestamptz
	HitlReviewNotes *string
	CreatedAt       pgtype.Timestamptz
	SubmittedByName *string
	ReviewedByName  *string
}

// GetWikiPageBySlug returns a wiki page by space slug + page slug.
func (s *Store) GetWikiPageBySlug(ctx context.Context, tenantID pgtype.UUID, spaceSlug, pageSlug string) (WikiPageWithSpace, error) {
	const q = `
SELECT wp.id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       sp.slug AS space_slug, sp.name AS space_name
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
WHERE sp.slug = $1 AND wp.slug = $2
  AND wp.is_active = true
LIMIT 1
`

	var p WikiPageWithSpace

	err := s.pool.Reader.QueryRow(ctx, q, spaceSlug, pageSlug).Scan(
		&p.ID, &p.SpaceID, &p.Slug, &p.Title, &p.Category,
		&p.IsActive, &p.IsFeatured, &p.IsLocked, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt, &p.IsJapabotIndexed,
		&p.SpaceSlug, &p.SpaceName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiPageWithSpace{}, pgx.ErrNoRows
		}

		return WikiPageWithSpace{}, fmt.Errorf("wiki: get page by slug: %w", err)
	}

	return p, nil
}

// ListWikiPagesBySpace returns all active wiki pages for a space, with the
// associated space slug and name (Rule 16 — avoid a second API round-trip).
func (s *Store) ListWikiPagesBySpace(ctx context.Context, tenantID pgtype.UUID, spaceSlug string) ([]WikiPageWithSpace, error) {
	const q = `
SELECT wp.id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       sp.slug AS space_slug, sp.name AS space_name
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
WHERE sp.slug = $1 AND wp.is_active = true
ORDER BY wp.category, wp.title
`

	rows, err := s.pool.Reader.Query(ctx, q, spaceSlug)
	if err != nil {
		return nil, fmt.Errorf("wiki: list pages by space: %w", err)
	}

	defer rows.Close()

	var out []WikiPageWithSpace

	for rows.Next() {
		var p WikiPageWithSpace
		if err := rows.Scan(
			&p.ID, &p.SpaceID, &p.Slug, &p.Title, &p.Category,
			&p.IsActive, &p.IsFeatured, &p.IsLocked, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt, &p.IsJapabotIndexed,
			&p.SpaceSlug, &p.SpaceName,
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

// ListWikiPagesBySpaceID returns all active wiki pages for a space by its ID.
// Used by the ?dest= geo-aware query parameter (ADR-086 geo-aware pattern).
func (s *Store) ListWikiPagesBySpaceID(ctx context.Context, tenantID, spaceID pgtype.UUID) ([]WikiPageWithSpace, error) {
	const q = `
SELECT wp.id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       sp.slug AS space_slug, sp.name AS space_name
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
WHERE wp.space_id = $1 AND wp.is_active = true
ORDER BY wp.category, wp.title`

	rows, err := s.pool.Reader.Query(ctx, q, spaceID)
	if err != nil {
		return nil, fmt.Errorf("wiki: list pages by space ID: %w", err)
	}

	defer rows.Close()

	var out []WikiPageWithSpace

	for rows.Next() {
		var p WikiPageWithSpace
		if err := rows.Scan(
			&p.ID, &p.SpaceID, &p.Slug, &p.Title, &p.Category,
			&p.IsActive, &p.IsFeatured, &p.IsLocked, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt, &p.IsJapabotIndexed,
			&p.SpaceSlug, &p.SpaceName,
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

var destinationLevels = []string{"country", "state", "city"}

func isDestinationLevel(level string) bool {
	for _, l := range destinationLevels {
		if l == level {
			return true
		}
	}

	return false
}

// ListWikiDestinations returns active destination spaces (country/state/city)
// with their active wiki page count. Optional level, parent and region filters
// narrow the result. When hasWiki is true, only spaces with at least one active
// wiki page are returned; otherwise all destination spaces are included.
func (s *Store) ListWikiDestinations(ctx context.Context, tenantID pgtype.UUID, level, parentSlug, region string, hasWiki bool) ([]WikiDestination, error) {
	var conds []string

	var args []any

	argn := 1

	conds = append(conds, "sp.status = 'active'")
	conds = append(conds, "sp.level IN ('country', 'state', 'city')")

	if isDestinationLevel(level) {
		conds = append(conds, fmt.Sprintf("sp.level = $%d", argn))
		args = append(args, level)
		argn++
	}

	if region != "" {
		conds = append(conds, fmt.Sprintf("sp.region = $%d", argn))
		args = append(args, region)
		argn++
	}

	var parentCond string
	if parentSlug != "" {
		parentCond = fmt.Sprintf("sp.parent_id = (SELECT id FROM spaces WHERE slug = $%d)", argn)

		args = append(args, parentSlug)
		argn++
	}

	where := strings.Join(conds, " AND ")
	if parentCond != "" {
		where += " AND " + parentCond
	}

	var q string
	if hasWiki {
		q = fmt.Sprintf(`
SELECT sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level, COUNT(wp.id) AS page_count
FROM spaces sp
JOIN wiki_pages wp ON wp.space_id = sp.id AND wp.is_active = true
WHERE %s
GROUP BY sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level
HAVING COUNT(wp.id) > 0
ORDER BY sp.name`, where)
	} else {
		q = fmt.Sprintf(`
SELECT sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level, COUNT(wp.id) FILTER (WHERE wp.is_active = true) AS page_count
FROM spaces sp
LEFT JOIN wiki_pages wp ON wp.space_id = sp.id
WHERE %s
GROUP BY sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level
ORDER BY sp.name`, where)
	}

	rows, err := s.pool.Reader.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("wiki: list destinations: %w", err)
	}
	defer rows.Close()

	var out []WikiDestination

	for rows.Next() {
		var d WikiDestination
		if err := rows.Scan(&d.SpaceID, &d.Slug, &d.Name, &d.GeoCode, &d.Region, &d.Level, &d.PageCount); err != nil {
			return nil, fmt.Errorf("wiki: scan destination: %w", err)
		}

		out = append(out, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("wiki: iterate destinations: %w", err)
	}

	return out, nil
}

// GetApprovedRevision returns the latest approved revision for a wiki page.
func (s *Store) GetApprovedRevision(ctx context.Context, pageID, tenantID pgtype.UUID) (WikiRevision, error) {
	const q = `
SELECT wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at, u.username AS submitted_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
WHERE wr.wiki_page_id = $1 AND wr.status = 'approved'
ORDER BY wr.created_at DESC
LIMIT 1
`

	var r WikiRevision

	err := s.pool.Reader.QueryRow(ctx, q, pageID, tenantID).Scan(
		&r.ID, &r.WikiPageID, &r.Content, &r.Summary,
		&r.Status, &r.SubmittedBy, &r.ReviewedBy, &r.ReviewedAt,
		&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
		&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
		&r.CreatedAt, &r.SubmittedByName,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
SELECT wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at, u.username AS submitted_by_name,
       ru.username AS reviewed_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
LEFT JOIN users ru ON ru.id = wr.reviewed_by
WHERE wr.wiki_page_id = $1
ORDER BY wr.created_at DESC
LIMIT $2 OFFSET $3
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
			&r.ID, &r.WikiPageID, &r.Content, &r.Summary,
			&r.Status, &r.SubmittedBy, &r.ReviewedBy, &r.ReviewedAt,
			&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
			&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
			&r.CreatedAt, &r.SubmittedByName, &r.ReviewedByName,
		); err != nil {
			return nil, 0, fmt.Errorf("wiki: scan revision: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("wiki: iterate revisions: %w", err)
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx, `SELECT COUNT(*) FROM wiki_revisions WHERE wiki_page_id = $1`, pageID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("wiki: count revisions: %w", err)
	}

	return out, total, nil
}

// ErrDuplicateContent is returned when a submitted revision has the same
// normalized content_hash as an existing revision for the same page (Rule 12).
// Callers should map this to a 409 duplicate_content API error.
var ErrDuplicateContent = errors.New("wiki: duplicate content")

// SubmitRevision inserts a new pending revision for a wiki page.
// BUG-002: computes content_hash (Rule 12) and rejects duplicate no-op edits
// with ErrDuplicateContent when the unique index idx_wiki_revisions_page_hash
// conflicts.
func (s *Store) SubmitRevision(ctx context.Context, tenantID, pageID, userID pgtype.UUID, body []byte, summary string, aiGenerated bool, aiModel, aiPromptHash string) (pgtype.UUID, error) {
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

	// BUG-002: compute content_hash per Rule 12. Extract plain text from the
	// TipTap JSON body, then hash with the standard content.ComputeContentHash.
	// Ref: japaconnect-engineering/AGENTS.md Rule 12
	plainText := rag.ExtractPlainText(string(body))
	contentHash := contentpkg.ComputeContentHash(plainText)

	err := s.pool.Writer.QueryRow(ctx, `
INSERT INTO wiki_revisions (wiki_page_id, content, summary, status,
    submitted_by, ai_generated, ai_model, ai_prompt_hash, content_hash) VALUES ($1, $2, $3, 'pending', $4, $5, $6, $7, $8)
RETURNING id
`, pageID, body, summaryPtr, userID, aiGenerated, aiModelPtr, aiPromptHashPtr, contentHash).Scan(&revID)
	if err != nil {
		// Map unique-violation on idx_wiki_revisions_page_hash to ErrDuplicateContent.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "wiki_revisions_page_hash") {
			return pgtype.UUID{}, ErrDuplicateContent
		}

		return pgtype.UUID{}, fmt.Errorf("wiki: submit revision: %w", err)
	}

	return revID, nil
}

// ApproveRevision marks a revision as approved and stamps the reviewer.
// Also updates the wiki_page's updated_at. Returns the approved revision's
// content (TipTap JSON) so the caller can enqueue RAG indexing (B8).
func (s *Store) ApproveRevision(ctx context.Context, revID, pageID, tenantID, reviewerID pgtype.UUID, reviewNotes string) ([]byte, error) {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("wiki: approve: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var notesPtr *string
	if reviewNotes != "" {
		notesPtr = &reviewNotes
	}

	var content []byte

	err = tx.QueryRow(ctx, `
UPDATE wiki_revisions
SET status = 'approved', reviewed_by = $2, reviewed_at = now(),
    hitl_reviewed_by = $2, hitl_reviewed_at = now(), hitl_review_notes = $3
WHERE id = $1 AND status = 'pending'
RETURNING content
`, revID, reviewerID, notesPtr).Scan(&content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("wiki: approve: revision not found or not pending")
		}

		return nil, fmt.Errorf("wiki: approve: update revision: %w", err)
	}

	_, err = tx.Exec(ctx, `
UPDATE wiki_pages SET updated_at = now() WHERE id = $1
`, pageID)
	if err != nil {
		return nil, fmt.Errorf("wiki: approve: update page: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("wiki: approve: commit: %w", err)
	}

	return content, nil
}

// RejectRevision marks a revision as rejected and stamps the reviewer.
func (s *Store) RejectRevision(ctx context.Context, revID, tenantID, reviewerID pgtype.UUID, reviewNotes string) error {
	var notesPtr *string
	if reviewNotes != "" {
		notesPtr = &reviewNotes
	}

	_, err := s.pool.Writer.Exec(ctx, `
UPDATE wiki_revisions
SET status = 'rejected', reviewed_by = $2, reviewed_at = now(),
    hitl_reviewed_by = $2, hitl_reviewed_at = now(), hitl_review_notes = $3
WHERE id = $1 AND status = 'pending'
`, revID, reviewerID, notesPtr)
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
	args = append(args, pageID)

	q := fmt.Sprintf(`
UPDATE wiki_pages SET %s WHERE id = $%d
`, strings.Join(conditions, ", "), argIdx)

	cmd, err := s.pool.Writer.Exec(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("wiki: update page: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return nil
}

// GetSpaceBySlug resolves a destination (country/state/city) space slug to its
// ID and level within a tenant. Returns pgx.ErrNoRows if the space does not
// exist or is not a destination space.
func (s *Store) GetSpaceBySlug(ctx context.Context, tenantID pgtype.UUID, spaceSlug string) (pgtype.UUID, string, error) {
	var id pgtype.UUID

	var level string

	err := s.pool.Reader.QueryRow(ctx, `
SELECT id, level
FROM spaces
WHERE slug = $1 AND level IN ('country', 'state', 'city') AND status = 'active'
LIMIT 1`, spaceSlug).Scan(&id, &level)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, "", pgx.ErrNoRows
		}

		return pgtype.UUID{}, "", fmt.Errorf("wiki: get space by slug: %w", err)
	}

	return id, level, nil
}

// GetSpaceByID resolves a destination (country/state/city) space ID to its
// slug and level within a tenant. Returns pgx.ErrNoRows if the space does not
// exist or is not a destination space.
func (s *Store) GetSpaceByID(ctx context.Context, tenantID, spaceID pgtype.UUID) (slug, level string, err error) {
	err = s.pool.Reader.QueryRow(ctx, `
SELECT slug, level
FROM spaces
WHERE id = $1 AND level IN ('country', 'state', 'city') AND status = 'active'
LIMIT 1`, spaceID).Scan(&slug, &level)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", pgx.ErrNoRows
		}

		return "", "", fmt.Errorf("wiki: get space by id: %w", err)
	}

	return slug, level, nil
}

// CreatePage creates a new wiki page. Used by the admin generate endpoint
// and the space activation seed job.
func (s *Store) CreatePage(ctx context.Context, tenantID, spaceID, createdBy pgtype.UUID, slug, title, category string) (pgtype.UUID, error) {
	var pageID pgtype.UUID

	err := s.pool.Writer.QueryRow(ctx, `
INSERT INTO wiki_pages (space_id, slug, title, category, created_by) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (space_id, slug) DO NOTHING
RETURNING id
`, spaceID, slug, title, category, createdBy).Scan(&pageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, ErrPageExists
		}

		return pgtype.UUID{}, fmt.Errorf("wiki: create page: %w", err)
	}

	return pageID, nil
}

// CreatePageWithRevision creates a new wiki page and its initial pending
// revision inside a single transaction.
func (s *Store) CreatePageWithRevision(ctx context.Context, tenantID, spaceID, userID pgtype.UUID, slug, title, category string, content []byte, summary string) (pageID, revID pgtype.UUID, err error) {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("wiki: create page tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert the page, ignoring duplicate (tenant_id, space_id, slug).
	var pageIDResult pgtype.UUID
	if err := tx.QueryRow(ctx, `
INSERT INTO wiki_pages (space_id, slug, title, category, created_by) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (space_id, slug) DO NOTHING
RETURNING id
`, spaceID, slug, title, category, userID).Scan(&pageIDResult); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgtype.UUID{}, ErrPageExists
		}

		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("wiki: create page: %w", err)
	}

	// Compute content_hash per Rule 12.
	plainText := rag.ExtractPlainText(string(content))
	contentHash := contentpkg.ComputeContentHash(plainText)

	var summaryPtr *string
	if summary != "" {
		summaryPtr = &summary
	}

	// Insert the initial pending revision.
	var revIDResult pgtype.UUID
	if err := tx.QueryRow(ctx, "\nINSERT INTO wiki_revisions (wiki_page_id, content, summary, status,\n    submitted_by, ai_generated, ai_model, ai_prompt_hash, content_hash)\nVALUES ($1, $2, $3, $4, 'pending', $5, false, NULL, $6)\nRETURNING", tenantID, pageIDResult, content, summaryPtr, userID, contentHash).Scan(&revIDResult); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "wiki_revisions_page_hash") {
			return pgtype.UUID{}, pgtype.UUID{}, ErrDuplicateContent
		}

		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("wiki: create page revision: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("wiki: create page commit: %w", err)
	}

	return pageIDResult, revIDResult, nil
}

// AdminListPagesFilter holds filter parameters for the admin page list.
type AdminListPagesFilter struct {
	TenantID pgtype.UUID
	SpaceID  pgtype.UUID
	Category string
	IsLocked *bool
	Q        string
	Limit    int32
	Offset   int32
}

// AdminListPages returns all wiki pages for admin management (paginated).
// Each row includes the latest revision's id, status, and an excerpt so the
// admin review queue can gate Approve/Reject actions (B1).
func (s *Store) AdminListPages(ctx context.Context, f AdminListPagesFilter) ([]AdminWikiPageWithSpace, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

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

	// BUG-021: build countArgs explicitly from the filter args (before
	// limit/offset are appended) rather than slicing args[:len-2], which
	// was brittle if the query builder changed.
	countArgs := make([]any, len(args))
	copy(countArgs, args)

	listQ := fmt.Sprintf(`
SELECT wp.id, wp.space_id, wp.slug, wp.title, wp.category,
       wp.is_active, wp.is_featured, wp.is_locked, wp.created_by,
       wp.created_at, wp.updated_at, wp.is_japabot_indexed,
       sp.slug AS space_slug, sp.name AS space_name,
       lr.id AS latest_revision_id, lr.status AS latest_revision_status,
       lr.excerpt
FROM wiki_pages wp
JOIN spaces sp ON sp.id = wp.space_id
LEFT JOIN LATERAL (
    SELECT wr.id, wr.status,
           COALESCE(wr.summary, left(btrim(regexp_replace(
               wr.content::text, '<[^>]+>', '', 'g')), 200), '') AS excerpt
    FROM wiki_revisions wr
    WHERE wr.wiki_page_id = wp.id
    ORDER BY wr.created_at DESC
    LIMIT 1
) lr ON true
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

	var out []AdminWikiPageWithSpace

	for rows.Next() {
		var p AdminWikiPageWithSpace
		if err := rows.Scan(
			&p.ID, &p.SpaceID, &p.Slug, &p.Title, &p.Category,
			&p.IsActive, &p.IsFeatured, &p.IsLocked, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt, &p.IsJapabotIndexed,
			&p.SpaceSlug, &p.SpaceName,
			&p.LatestRevisionID, &p.LatestRevisionStatus, &p.Excerpt,
		); err != nil {
			return nil, 0, fmt.Errorf("wiki: admin scan page: %w", err)
		}

		out = append(out, p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("wiki: admin iterate pages: %w", err)
	}

	countQ := "SELECT COUNT(*) FROM wiki_pages wp WHERE " + whereClause

	var total int64

	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("wiki: admin count pages: %w", err)
	}

	return out, total, nil
}

// ErrPageExists is returned when a wiki page with the same slug already
// exists in the same space.
var ErrPageExists = errors.New("wiki page already exists")
