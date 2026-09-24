// Package wiki implements the Destination Wiki (§80, ADR-088).
// // Collaborative, versioned knowledge-base pages attached to each destination
// country/city Space. Wiki pages are living documents continuously improved
// by reputation-gated community members. Every edit creates a pending
// revision that must be approved by a moderator+ before it becomes the
// current content.
// // Ref: japaconnect-engineering/sections/80-destination-wiki.md
// //	japaconnect-engineering/adr/ADR-088-destination-wiki.md
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
// //nolint:revive // exported: stutter naming is intentional
type WikiPage struct {
	ID pgtype.UUID

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
// //nolint:revive // exported: stutter naming is intentional
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
// //nolint:revive // exported: stutter naming is intentional
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
// //nolint:revive // exported: stutter naming is intentional
type WikiRevision struct {
	ID pgtype.UUID

	WikiPageID pgtype.UUID // nullable — page-less ops revisions (§80)
	Content    []byte      // TipTap JSON
	Summary    *string
	// §80 community field ops: kind is content|fields|mixed; ops carries the
	// structured field/group mutation array; target_* identifies the
	// polymorphic entity the ops apply to.
	Kind             string
	Ops              []byte
	TargetEntityType *string
	TargetEntityID   pgtype.UUID

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
func (s *Store) GetWikiPageBySlug(ctx context.Context, spaceSlug, pageSlug string) (WikiPageWithSpace, error) {
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
func (s *Store) ListWikiPagesBySpace(ctx context.Context, spaceSlug string) ([]WikiPageWithSpace, error) {
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
func (s *Store) ListWikiPagesBySpaceID(ctx context.Context, spaceID pgtype.UUID) ([]WikiPageWithSpace, error) {
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

// destinationLevels are the space levels that may carry wiki pages. Geo
// destinations (country/state/city) plus university spaces (§80 amendment —
// University Spaces).
var destinationLevels = []string{"country", "state", "city", "university"}

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
func (s *Store) ListWikiDestinations(ctx context.Context, level, parentSlug, region, search string, hasWiki bool) ([]WikiDestination, error) {
	var conds []string

	var args []any

	argn := 1

	conds = append(conds, "sp.status = 'active'")
	// University spaces are typed via space_types.slug = 'university' — most
	// carry level = 'custom' — so the base set is geo levels plus the two
	// legacy level='university' rows; typed universities enter via the
	// explicit ?level=university filter or has_wiki below.
	conds = append(conds, "(sp.level IN ('country', 'state', 'city', 'university') OR st.slug = 'university')")

	if isDestinationLevel(level) {
		if level == "university" {
			// Match by space type so level='custom' universities are included.
			conds = append(conds, fmt.Sprintf("(sp.level = $%d OR st.slug = $%d)", argn, argn))
		} else {
			conds = append(conds, fmt.Sprintf("sp.level = $%d", argn))
		}
		args = append(args, level)
		argn++
	}

	if region != "" {
		conds = append(conds, fmt.Sprintf("sp.region = $%d", argn))
		args = append(args, region)
		argn++
	}

	if search != "" {
		conds = append(conds, fmt.Sprintf("sp.name ILIKE '%%' || $%d || '%%'", argn))
		args = append(args, search)
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
LEFT JOIN space_types st ON st.id = sp.space_type_id
JOIN wiki_pages wp ON wp.space_id = sp.id AND wp.is_active = true
WHERE %s
GROUP BY sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level
HAVING COUNT(wp.id) > 0
ORDER BY sp.name`, where)
	} else {
		q = fmt.Sprintf(`
SELECT sp.id, sp.slug, sp.name, sp.geo_code, sp.region, sp.level, COUNT(wp.id) FILTER (WHERE wp.is_active = true) AS page_count
FROM spaces sp
LEFT JOIN space_types st ON st.id = sp.space_type_id
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
// kind='fields' revisions are excluded: they carry no prose (content='{}') and
// approving one must never replace the page's body (§80 community field ops).
func (s *Store) GetApprovedRevision(ctx context.Context, pageID pgtype.UUID) (WikiRevision, error) {
	const q = `
SELECT wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.kind, wr.ops, wr.target_entity_type, wr.target_entity_id,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at, u.username AS submitted_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
WHERE wr.wiki_page_id = $1 AND wr.status = 'approved'
  AND wr.kind IN ('content', 'mixed')
ORDER BY wr.created_at DESC
LIMIT 1
`

	var r WikiRevision

	err := s.pool.Reader.QueryRow(ctx, q, pageID).Scan(
		&r.ID, &r.WikiPageID, &r.Content, &r.Summary,
		&r.Kind, &r.Ops, &r.TargetEntityType, &r.TargetEntityID,
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
func (s *Store) ListRevisions(ctx context.Context, pageID pgtype.UUID, limit, offset int32) ([]WikiRevision, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	const q = `
SELECT wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.kind, wr.ops, wr.target_entity_type, wr.target_entity_id,
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

	rows, err := s.pool.Reader.Query(ctx, q, pageID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: list revisions: %w", err)
	}

	defer rows.Close()

	var out []WikiRevision

	for rows.Next() {
		var r WikiRevision
		if err := rows.Scan(
			&r.ID, &r.WikiPageID, &r.Content, &r.Summary,
			&r.Kind, &r.Ops, &r.TargetEntityType, &r.TargetEntityID,
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

// revisionContentHash computes the dedupe hash for a revision (Rule 12):
// plain text extracted from the TipTap body plus a NUL-separated ops payload.
// Appending the ops bytes keeps idx_wiki_revisions_page_hash deduping
// identical submissions while preventing fields-only revisions (empty prose)
// from all colliding on the hash of an empty document.
// Ref: japaconnect-engineering/AGENTS.md Rule 12
func revisionContentHash(body, ops []byte) string {
	// Normalize here (not just in insertRevision) so every caller hashes the
	// same input whether ops arrives as nil or []byte("[]").
	if len(ops) == 0 {
		ops = []byte("[]")
	}

	plainText := rag.ExtractPlainText(string(body))

	return contentpkg.ComputeContentHash(plainText + "\x00" + string(ops))
}

// SubmitRevision inserts a new pending revision. pageID may be an invalid
// pgtype.UUID for page-less ops revisions (§80 community field ops); kind is
// content|fields|mixed; body may be empty for fields-only revisions (stored
// as '{}'::jsonb); ops is the marshaled FieldOp array ('[]' when nil).
// BUG-002: computes content_hash (Rule 12) and rejects duplicate no-op edits
// with ErrDuplicateContent when the unique index idx_wiki_revisions_page_hash
// conflicts.
func (s *Store) SubmitRevision(ctx context.Context, pageID, userID pgtype.UUID, kind string, body, ops []byte, summary string, targetType *string, targetID pgtype.UUID) (pgtype.UUID, error) {
	return s.insertRevision(ctx, pageID, userID, kind, body, ops, summary, targetType, targetID, false, "", "")
}

// SubmitAIRevision inserts a pending revision flagged as AI-generated
// (ai_generated, ai_model, ai_prompt_hash) — used by GenerateDraft
// (ADR-088 HITL pipeline). Ops revisions never carry AI provenance, so this
// always records kind='content' with an empty ops array.
func (s *Store) SubmitAIRevision(ctx context.Context, pageID, userID pgtype.UUID, body []byte, summary, aiModel, aiPromptHash string) (pgtype.UUID, error) {
	return s.insertRevision(ctx, pageID, userID, "content", body, nil, summary, nil, pgtype.UUID{}, true, aiModel, aiPromptHash)
}

// insertRevision is the shared INSERT for SubmitRevision/SubmitAIRevision.
func (s *Store) insertRevision(ctx context.Context, pageID, userID pgtype.UUID, kind string, body, ops []byte, summary string, targetType *string, targetID pgtype.UUID, aiGenerated bool, aiModel, aiPromptHash string) (pgtype.UUID, error) {
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

	if kind == "" {
		kind = "content"
	}

	// content is NOT NULL — fields-only revisions store an empty doc so
	// ::text excerpt rendering keeps working.
	if len(body) == 0 {
		body = []byte("{}")
	}

	// ops is NOT NULL DEFAULT '[]' — normalize nil to an empty array so the
	// content_hash input is stable whether callers pass nil or []byte("[]").
	if len(ops) == 0 {
		ops = []byte("[]")
	}

	// BUG-002: compute content_hash per Rule 12 (see revisionContentHash).
	contentHash := revisionContentHash(body, ops)

	err := s.pool.Writer.QueryRow(ctx, `
INSERT INTO wiki_revisions (wiki_page_id, kind, content, ops, summary,
    target_entity_type, target_entity_id, status,
    submitted_by, ai_generated, ai_model, ai_prompt_hash, content_hash)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'pending', $8, $9, $10, $11, $12)
RETURNING id
`, pageID, kind, body, ops, summaryPtr, targetType, targetID,
		userID, aiGenerated, aiModelPtr, aiPromptHashPtr, contentHash).Scan(&revID)
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
func (s *Store) ApproveRevision(ctx context.Context, revID, pageID, reviewerID pgtype.UUID, reviewNotes string) ([]byte, error) {
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
func (s *Store) RejectRevision(ctx context.Context, revID, reviewerID pgtype.UUID, reviewNotes string) error {
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

// revisionColumns is the canonical SELECT column list materializing a
// WikiRevision (scan order must match the struct fills below).
const revisionColumns = `wr.id, wr.wiki_page_id, wr.content, wr.summary,
       wr.kind, wr.ops, wr.target_entity_type, wr.target_entity_id,
       wr.status, wr.submitted_by, wr.reviewed_by, wr.reviewed_at,
       wr.ai_generated, wr.ai_model, wr.ai_prompt_hash,
       wr.hitl_reviewed_by, wr.hitl_reviewed_at, wr.hitl_review_notes,
       wr.created_at`

// scanRevision scans one row produced by revisionColumns (plus optional
// trailing username columns handled by callers) into a WikiRevision.
func scanRevision(r *WikiRevision, row interface{ Scan(...any) error }, extra ...any) error {
	dest := []any{
		&r.ID, &r.WikiPageID, &r.Content, &r.Summary,
		&r.Kind, &r.Ops, &r.TargetEntityType, &r.TargetEntityID,
		&r.Status, &r.SubmittedBy, &r.ReviewedBy, &r.ReviewedAt,
		&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
		&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
		&r.CreatedAt,
	}

	return row.Scan(append(dest, extra...)...)
}

// GetRevision returns a single revision row by ID (any kind/status), with
// submitter/reviewer usernames resolved like ListRevisions.
func (s *Store) GetRevision(ctx context.Context, revID pgtype.UUID) (WikiRevision, error) {
	q := `
SELECT ` + revisionColumns + `,
       u.username AS submitted_by_name, ru.username AS reviewed_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
LEFT JOIN users ru ON ru.id = wr.reviewed_by
WHERE wr.id = $1
`

	var r WikiRevision

	err := scanRevision(&r, s.pool.Reader.QueryRow(ctx, q, revID), &r.SubmittedByName, &r.ReviewedByName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiRevision{}, pgx.ErrNoRows
		}

		return WikiRevision{}, fmt.Errorf("wiki: get revision: %w", err)
	}

	return r, nil
}

// BeginTx opens a writer-pool transaction for multi-statement wiki writes —
// used by the service's ops-approval path where the revision status flip and
// the field-ops application must commit or roll back atomically (§80).
func (s *Store) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Writer.Begin(ctx)
}

// ApproveRevisionByID marks a revision as approved by revision ID alone —
// used by the revision-keyed review routes where the caller may not know the
// host page (page-less ops revisions have NULL wiki_page_id, §80). Returns
// the full approved row (kind + ops + content) so the caller can apply ops
// and enqueue propagation. The host page's updated_at is bumped when the
// revision is page-bound.
func (s *Store) ApproveRevisionByID(ctx context.Context, revID, reviewerID pgtype.UUID, reviewNotes string) (WikiRevision, error) {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return WikiRevision{}, fmt.Errorf("wiki: approve: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	r, err := s.ApproveRevisionTx(ctx, tx, revID, reviewerID, reviewNotes)
	if err != nil {
		return WikiRevision{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WikiRevision{}, fmt.Errorf("wiki: approve: commit: %w", err)
	}

	return r, nil
}

// ApproveRevisionTx is the transaction-scoped core of ApproveRevisionByID.
// Callers supplying their own transaction (e.g. the service's
// approve+applyOps path in service_fields.go) share one atomic commit between
// the status flip and the field mutations. Marks the revision approved (it
// must be pending), stamps the reviewer, bumps the host page's updated_at
// when the revision is page-bound, and returns the updated row.
func (s *Store) ApproveRevisionTx(ctx context.Context, tx pgx.Tx, revID, reviewerID pgtype.UUID, reviewNotes string) (WikiRevision, error) {
	var notesPtr *string
	if reviewNotes != "" {
		notesPtr = &reviewNotes
	}

	var r WikiRevision

	err := tx.QueryRow(ctx, `
UPDATE wiki_revisions
SET status = 'approved', reviewed_by = $2, reviewed_at = now(),
    hitl_reviewed_by = $2, hitl_reviewed_at = now(), hitl_review_notes = $3
WHERE id = $1 AND status = 'pending'
RETURNING id, wiki_page_id, content, summary,
    kind, ops, target_entity_type, target_entity_id,
    status, submitted_by, reviewed_by, reviewed_at,
    ai_generated, ai_model, ai_prompt_hash,
    hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes,
    created_at
`, revID, reviewerID, notesPtr).Scan(
		&r.ID, &r.WikiPageID, &r.Content, &r.Summary,
		&r.Kind, &r.Ops, &r.TargetEntityType, &r.TargetEntityID,
		&r.Status, &r.SubmittedBy, &r.ReviewedBy, &r.ReviewedAt,
		&r.AiGenerated, &r.AiModel, &r.AiPromptHash,
		&r.HitlReviewedBy, &r.HitlReviewedAt, &r.HitlReviewNotes,
		&r.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WikiRevision{}, errors.New("wiki: approve: revision not found or not pending")
		}

		return WikiRevision{}, fmt.Errorf("wiki: approve: update revision: %w", err)
	}

	if r.WikiPageID.Valid {
		if _, err := tx.Exec(ctx, `
UPDATE wiki_pages SET updated_at = now() WHERE id = $1
`, r.WikiPageID); err != nil {
			return WikiRevision{}, fmt.Errorf("wiki: approve: update page: %w", err)
		}
	}

	return r, nil
}

// RejectRevisionByID marks a revision as rejected by revision ID alone —
// the revision-keyed counterpart used by the admin review queue (§80).
// RejectRevision already keys on revision ID, so this delegates to it.
func (s *Store) RejectRevisionByID(ctx context.Context, revID, reviewerID pgtype.UUID, reviewNotes string) error {
	return s.RejectRevision(ctx, revID, reviewerID, reviewNotes)
}

// ListRevisionsByTarget returns revisions (all statuses, newest first)
// targeting a specific entity via target_entity_type + target_entity_id.
func (s *Store) ListRevisionsByTarget(ctx context.Context, entityType string, entityID pgtype.UUID, limit, offset int32) ([]WikiRevision, int64, error) {
	if limit <= 0 {
		limit = 20
	}

	if limit > 100 {
		limit = 100
	}

	q := `
SELECT ` + revisionColumns + `,
       u.username AS submitted_by_name, ru.username AS reviewed_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
LEFT JOIN users ru ON ru.id = wr.reviewed_by
WHERE wr.target_entity_type = $1 AND wr.target_entity_id = $2
ORDER BY wr.created_at DESC
LIMIT $3 OFFSET $4
`

	rows, err := s.pool.Reader.Query(ctx, q, entityType, entityID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: list revisions by target: %w", err)
	}

	defer rows.Close()

	var out []WikiRevision

	for rows.Next() {
		var r WikiRevision
		if err := scanRevision(&r, rows, &r.SubmittedByName, &r.ReviewedByName); err != nil {
			return nil, 0, fmt.Errorf("wiki: scan revision: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("wiki: iterate revisions by target: %w", err)
	}

	var total int64
	if err := s.pool.Reader.QueryRow(ctx, `
SELECT COUNT(*) FROM wiki_revisions
WHERE target_entity_type = $1 AND target_entity_id = $2
`, entityType, entityID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("wiki: count revisions by target: %w", err)
	}

	return out, total, nil
}

// AdminListRevisions returns revisions for the admin review queue (newest
// first). kind and status are optional filters — an empty string means no
// filter (§80 community field ops review).
func (s *Store) AdminListRevisions(ctx context.Context, kind, status string, limit, offset int32) ([]WikiRevision, int64, error) {
	var (
		conditions []string
		args       []any
		argIdx     = 1
	)

	if kind != "" {
		conditions = append(conditions, fmt.Sprintf("wr.kind = $%d", argIdx))
		args = append(args, kind)
		argIdx++
	}

	if status != "" {
		conditions = append(conditions, fmt.Sprintf("wr.status = $%d", argIdx))
		args = append(args, status)
		argIdx++
	}

	if limit <= 0 {
		limit = 20
	}

	if limit > 200 {
		limit = 200
	}

	if offset < 0 {
		offset = 0
	}

	whereClause := "true"
	if len(conditions) > 0 {
		whereClause = strings.Join(conditions, " AND ")
	}

	countArgs := make([]any, len(args))
	copy(countArgs, args)

	listQ := fmt.Sprintf(`
SELECT %s,
       u.username AS submitted_by_name, ru.username AS reviewed_by_name
FROM wiki_revisions wr
LEFT JOIN users u ON u.id = wr.submitted_by
LEFT JOIN users ru ON ru.id = wr.reviewed_by
WHERE %s
ORDER BY wr.created_at DESC
LIMIT $%d OFFSET $%d
`, revisionColumns, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := s.pool.Reader.Query(ctx, listQ, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("wiki: admin list revisions: %w", err)
	}
	defer rows.Close()

	var out []WikiRevision

	for rows.Next() {
		var r WikiRevision
		if err := scanRevision(&r, rows, &r.SubmittedByName, &r.ReviewedByName); err != nil {
			return nil, 0, fmt.Errorf("wiki: admin scan revision: %w", err)
		}

		out = append(out, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("wiki: admin iterate revisions: %w", err)
	}

	var total int64

	countQ := "SELECT COUNT(*) FROM wiki_revisions wr WHERE " + whereClause
	if err := s.pool.Reader.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("wiki: admin count revisions: %w", err)
	}

	return out, total, nil
}

// CountPendingByTarget returns the number of pending revisions targeting an
// entity — used to surface "N pending" badges on the entity fields panel.
func (s *Store) CountPendingByTarget(ctx context.Context, entityType string, entityID pgtype.UUID) (int64, error) {
	var n int64

	err := s.pool.Reader.QueryRow(ctx, `
SELECT COUNT(*) FROM wiki_revisions
WHERE target_entity_type = $1 AND target_entity_id = $2 AND status = 'pending'
`, entityType, entityID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("wiki: count pending by target: %w", err)
	}

	return n, nil
}

// FindHostPageID returns the first active wiki page for a space+category —
// the "host page" an entity-bound ops revision attaches to when the entity
// is space-backed (§80). Returns pgx.ErrNoRows when no page matches.
func (s *Store) FindHostPageID(ctx context.Context, spaceID pgtype.UUID, category string) (pgtype.UUID, error) {
	var id pgtype.UUID

	err := s.pool.Reader.QueryRow(ctx, `
SELECT id FROM wiki_pages
WHERE space_id = $1 AND category = $2 AND is_active = true
ORDER BY created_at ASC
LIMIT 1
`, spaceID, category).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgx.ErrNoRows
		}

		return pgtype.UUID{}, fmt.Errorf("wiki: find host page: %w", err)
	}

	return id, nil
}

// GetSpaceTypeID returns the space_type_id for a space — needed to resolve
// entity_class for space-backed field group assignments (§06/§80).
func (s *Store) GetSpaceTypeID(ctx context.Context, spaceID pgtype.UUID) (pgtype.UUID, error) {
	var id pgtype.UUID

	err := s.pool.Reader.QueryRow(ctx, `
SELECT space_type_id FROM spaces WHERE id = $1
`, spaceID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, pgx.ErrNoRows
		}

		return pgtype.UUID{}, fmt.Errorf("wiki: get space type id: %w", err)
	}

	return id, nil
}

// UpdatePage updates wiki page admin-managed fields (lock, feature, active).
func (s *Store) UpdatePage(ctx context.Context, pageID pgtype.UUID, isLocked, isFeatured, isActive *bool) error {
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

// GetSpaceBySlug resolves a destination (country/state/city/university) space
// slug to its ID and level within a platform. Returns pgx.ErrNoRows if the
// space does not exist or is not a destination space.
func (s *Store) GetSpaceBySlug(ctx context.Context, spaceSlug string) (pgtype.UUID, string, error) {
	var id pgtype.UUID

	var level string

	// University spaces are typed via space_types.slug = 'university' — most
	// carry level = 'custom', so matching on level alone would miss them.
	err := s.pool.Reader.QueryRow(ctx, `
SELECT sp.id, sp.level
FROM spaces sp
LEFT JOIN space_types st ON st.id = sp.space_type_id
WHERE sp.slug = $1 AND sp.status = 'active'
  AND (sp.level IN ('country', 'state', 'city', 'university') OR st.slug = 'university')
LIMIT 1`, spaceSlug).Scan(&id, &level)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, "", pgx.ErrNoRows
		}

		return pgtype.UUID{}, "", fmt.Errorf("wiki: get space by slug: %w", err)
	}

	return id, level, nil
}

// IsUniversitySpaceBySlug reports whether the slug resolves to an active
// university space — typed via space_types.slug = 'university' or, for the
// seeded university spaces, level = 'university'. Unknown slugs return false.
func (s *Store) IsUniversitySpaceBySlug(ctx context.Context, spaceSlug string) (bool, error) {
	var isUni bool
	err := s.pool.Reader.QueryRow(ctx, `
SELECT (sp.level = 'university' OR st.slug = 'university')
FROM spaces sp
LEFT JOIN space_types st ON st.id = sp.space_type_id
WHERE sp.slug = $1 AND sp.status = 'active'
LIMIT 1`, spaceSlug).Scan(&isUni)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}

		return false, fmt.Errorf("wiki: is university space: %w", err)
	}

	return isUni, nil
}

// GetSpaceByID resolves a destination (country/state/city/university) space ID
// to its slug and level within a platform. Returns pgx.ErrNoRows if the space
// does not exist or is not a destination space.
func (s *Store) GetSpaceByID(ctx context.Context, spaceID pgtype.UUID) (slug, level string, err error) {
	err = s.pool.Reader.QueryRow(ctx, `
SELECT sp.slug, sp.level
FROM spaces sp
LEFT JOIN space_types st ON st.id = sp.space_type_id
WHERE sp.id = $1 AND sp.status = 'active'
  AND (sp.level IN ('country', 'state', 'city', 'university') OR st.slug = 'university')
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
func (s *Store) CreatePage(ctx context.Context, spaceID, createdBy pgtype.UUID, slug, title, category string) (pgtype.UUID, error) {
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
func (s *Store) CreatePageWithRevision(ctx context.Context, spaceID, userID pgtype.UUID, slug, title, category string, content []byte, summary string) (pageID, revID pgtype.UUID, err error) {
	tx, err := s.pool.Writer.Begin(ctx)
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, fmt.Errorf("wiki: create page tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert the page, ignoring duplicate ( space_id, slug).
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

	// Compute content_hash per Rule 12 — same pipeline as insertRevision
	// (plain text + NUL + ops) so a later identical SubmitRevision still
	// dedupes against this initial revision.
	contentHash := revisionContentHash(content, nil)

	var summaryPtr *string
	if summary != "" {
		summaryPtr = &summary
	}

	// Insert the initial pending revision.
	var revIDResult pgtype.UUID
	if err := tx.QueryRow(ctx, `
INSERT INTO wiki_revisions (wiki_page_id, kind, content, ops, summary,
    target_entity_type, target_entity_id, status,
    submitted_by, ai_generated, ai_model, ai_prompt_hash, content_hash)
VALUES ($1, 'content', $2, '[]'::jsonb, $3, NULL, NULL, 'pending', $4, false, NULL, NULL, $5)
RETURNING id
`, pageIDResult, content, summaryPtr, userID, contentHash).Scan(&revIDResult); err != nil {
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
