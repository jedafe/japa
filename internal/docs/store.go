package docs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// mapWriteErr translates Postgres constraint violations into sentinel errors
// so handlers emit a 409 instead of a raw 500 (§44 amendment 2).
func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return fmt.Errorf("%w: %s", errpkg.ErrDuplicate, pgErr.ConstraintName)
		}
		if pgErr.Code == "23503" {
			return fmt.Errorf("%w: %s", errpkg.ErrInvalid, pgErr.ConstraintName)
		}
	}

	return err
}

// Store provides low-level database access for documentation pages.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
	// pool is kept for multi-statement writes (page write + revision
	// snapshot commit together, §44 amendment 5). Nil in unit tests —
	// callers then fall back to sequential writes.
	pool *db.DB
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// docPageRow is a doc page plus the media join columns the per-page
// feature image resolves from (§44 amendment 5).
type docPageRow struct {
	gen.DocPage
	FeatureImageBucket *string
	FeatureImagePath   *string
}

// BeginTx opens a writer-pool transaction for the page write + revision
// snapshot — they must commit or roll back together (§44 amendment 5).
func (s *Store) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Writer.Begin(ctx)
}

// getBySlug returns a published doc page by audience and slug.
func (s *Store) getBySlug(ctx context.Context, audience, slug string) (docPageRow, error) {
	row, err := s.read.GetDocPageBySlug(ctx, gen.GetDocPageBySlugParams{
		Audience: audience,
		Slug:     slug,
	})
	if err != nil {
		return docPageRow{}, fmt.Errorf("getting doc page by slug: %w", err)
	}

	return docPageRow{DocPage: gen.DocPage{
		ID: row.ID,

		Title:          row.Title,
		Slug:           row.Slug,
		Audience:       row.Audience,
		Content:        row.Content,
		AiGenerated:    row.AiGenerated,
		ReviewStatus:   row.ReviewStatus,
		LastReviewedBy: row.LastReviewedBy,
		LastReviewedAt: row.LastReviewedAt,
		ParentID:       row.ParentID,
		Position:       row.Position,
		Category:       row.Category,
		Subcategory:    row.Subcategory,
		HelpfulYes:     row.HelpfulYes,
		HelpfulNo:      row.HelpfulNo,
		FeatureImageID: row.FeatureImageID,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, FeatureImageBucket: row.FeatureImageBucket, FeatureImagePath: row.FeatureImagePath}, nil
}

// getBySlugAnyStatus returns a doc page by audience+slug regardless of
// review status. Editor preview only — callers must gate on docs.admin
// (§44 amendment 2).
func (s *Store) getBySlugAnyStatus(ctx context.Context, audience, slug string) (docPageRow, error) {
	row, err := s.read.GetDocPageBySlugAnyStatus(ctx, gen.GetDocPageBySlugAnyStatusParams{
		Audience: audience,
		Slug:     slug,
	})
	if err != nil {
		return docPageRow{}, fmt.Errorf("getting doc page by slug (any status): %w", err)
	}

	return docPageRow{DocPage: gen.DocPage{
		ID:             row.ID,
		Title:          row.Title,
		Slug:           row.Slug,
		Audience:       row.Audience,
		Content:        row.Content,
		AiGenerated:    row.AiGenerated,
		AiModel:        row.AiModel,
		AiGeneratedAt:  row.AiGeneratedAt,
		HitlReviewedBy: row.HitlReviewedBy,
		HitlReviewedAt: row.HitlReviewedAt,
		ReviewStatus:   row.ReviewStatus,
		LastReviewedBy: row.LastReviewedBy,
		LastReviewedAt: row.LastReviewedAt,
		ParentID:       row.ParentID,
		Position:       row.Position,
		Category:       row.Category,
		Subcategory:    row.Subcategory,
		HelpfulYes:     row.HelpfulYes,
		HelpfulNo:      row.HelpfulNo,
		FeatureImageID: row.FeatureImageID,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, FeatureImageBucket: row.FeatureImageBucket, FeatureImagePath: row.FeatureImagePath}, nil
}

// getByID returns a doc page by ID.
func (s *Store) getByID(ctx context.Context, id pgtype.UUID) (docPageRow, error) {
	row, err := s.read.GetDocPageByID(ctx,
		id,
	)
	if err != nil {
		return docPageRow{}, fmt.Errorf("getting doc page by ID: %w", err)
	}

	return docPageRow{DocPage: gen.DocPage{
		ID: row.ID,

		Title:          row.Title,
		Slug:           row.Slug,
		Audience:       row.Audience,
		Content:        row.Content,
		AiGenerated:    row.AiGenerated,
		ReviewStatus:   row.ReviewStatus,
		LastReviewedBy: row.LastReviewedBy,
		LastReviewedAt: row.LastReviewedAt,
		ParentID:       row.ParentID,
		Position:       row.Position,
		Category:       row.Category,
		Subcategory:    row.Subcategory,
		HelpfulYes:     row.HelpfulYes,
		HelpfulNo:      row.HelpfulNo,
		FeatureImageID: row.FeatureImageID,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		DeletedAt:      row.DeletedAt,
		AiModel:        row.AiModel,
		AiGeneratedAt:  row.AiGeneratedAt,
		HitlReviewedBy: row.HitlReviewedBy,
		HitlReviewedAt: row.HitlReviewedAt,
	}, FeatureImageBucket: row.FeatureImageBucket, FeatureImagePath: row.FeatureImagePath}, nil
}

// listByAudience returns published doc pages for an audience.
func (s *Store) listByAudience(ctx context.Context, audience string) ([]gen.ListDocPagesByAudienceRow, error) {
	rows, err := s.read.ListDocPagesByAudience(ctx,

		audience,
	)
	if err != nil {
		return nil, fmt.Errorf("listing doc pages by audience: %w", err)
	}

	return rows, nil
}

// listAllByAudience returns all doc pages for an audience, optionally
// filtered to one review_status ("" = any status). showDeleted swaps the
// live-page view for the trash view (Rule 11).
func (s *Store) listAllByAudience(ctx context.Context, audience, status string, showDeleted bool, limit, offset int32) ([]gen.ListAllDocPagesByAudienceRow, error) {
	rows, err := s.read.ListAllDocPagesByAudience(ctx, gen.ListAllDocPagesByAudienceParams{
		Audience:     audience,
		StatusFilter: status,
		ShowDeleted:  showDeleted,
		ResultLimit:  limit,
		ResultOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing all doc pages by audience: %w", err)
	}

	return rows, nil
}

// adminListFilter carries the parsed GET /admin/docs/pages parameters.
type adminListFilter struct {
	// Audiences is the gate-resolved set of audiences the caller may see —
	// pushed to SQL so counts and pages never include unreadable rows.
	Audiences   []string
	Status      string
	ShowDeleted bool
	Search      string
	Sort        string
	Desc        bool
	Limit       int32
	Offset      int32
}

// adminListSortColumns maps client sort keys to ORDER BY expressions. The
// whitelist is exhaustive — user input is never interpolated.
var adminListSortColumns = map[string]string{
	"title":      "p.title",
	"slug":       "p.slug",
	"audience":   "p.audience",
	"position":   "p.position",
	"status":     "p.review_status",
	"created_at": "p.created_at",
	"updated_at": "p.updated_at",
}

// listAdminPaged returns the filtered+sorted page slice plus the total row
// count under the same filters, so the admin table can render accurate
// pagination metadata.
func (s *Store) listAdminPaged(ctx context.Context, f adminListFilter) ([]docPageRow, int64, error) {
	conds := []string{"($1::boolean = (p.deleted_at IS NOT NULL))"}
	args := []any{f.ShowDeleted}

	args = append(args, f.Audiences)
	conds = append(conds, fmt.Sprintf("p.audience = ANY($%d::text[])", len(args)))

	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, fmt.Sprintf("p.review_status = $%d", len(args)))
	}
	if f.Search != "" {
		args = append(args, "%"+likeEscaper.Replace(f.Search)+"%")
		conds = append(conds, fmt.Sprintf("(p.title ILIKE $%d OR p.slug ILIKE $%d)", len(args), len(args)))
	}

	orderBy := "p.audience ASC, p.position ASC, p.title ASC"
	if col, ok := adminListSortColumns[f.Sort]; ok {
		dir := "ASC"
		if f.Desc {
			dir = "DESC"
		}
		orderBy = col + " " + dir + ", p.title ASC"
	}

	args = append(args, f.Limit)
	limIdx := len(args)
	args = append(args, f.Offset)
	offIdx := len(args)

	query := fmt.Sprintf(`SELECT p.id, p.title, p.slug, p.audience, p.ai_generated, p.ai_model,
  p.ai_generated_at, p.hitl_reviewed_by, p.hitl_reviewed_at, p.review_status,
  p.last_reviewed_by, p.last_reviewed_at, p.parent_id, p.position, p.category, p.subcategory,
  p.feature_image_id,
  m.storage_bucket AS feature_image_bucket, m.storage_path AS feature_image_path,
  p.created_at, p.updated_at, p.deleted_at, count(*) OVER() AS total
FROM doc_pages p
LEFT JOIN media m ON m.id = p.feature_image_id
WHERE %s
ORDER BY %s
LIMIT NULLIF($%d::int, 0) OFFSET COALESCE($%d::int, 0)`,
		strings.Join(conds, "\n  AND "), orderBy, limIdx, offIdx)

	rows, err := s.pool.Reader.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing admin doc pages: %w", err)
	}
	defer rows.Close()

	out := make([]docPageRow, 0, f.Limit)
	var total int64
	for rows.Next() {
		var r docPageRow
		var n int64
		if err := rows.Scan(
			&r.ID, &r.Title, &r.Slug, &r.Audience, &r.AiGenerated, &r.AiModel,
			&r.AiGeneratedAt, &r.HitlReviewedBy, &r.HitlReviewedAt, &r.ReviewStatus,
			&r.LastReviewedBy, &r.LastReviewedAt, &r.ParentID, &r.Position, &r.Category, &r.Subcategory,
			&r.FeatureImageID, &r.FeatureImageBucket, &r.FeatureImagePath,
			&r.CreatedAt, &r.UpdatedAt, &r.DeletedAt, &n,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning admin doc page: %w", err)
		}
		total = n
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterating admin doc pages: %w", err)
	}

	return out, total, nil
}

// listSummaries returns published summaries across the given (already
// gate-filtered) audiences — one aggregate query for the docs landing
// changelog, replacing the per-audience fan-out.
func (s *Store) listSummaries(ctx context.Context, audiences []string) ([]gen.ListDocSummariesRow, error) {
	rows, err := s.read.ListDocSummaries(ctx, audiences)
	if err != nil {
		return nil, fmt.Errorf("listing doc summaries: %w", err)
	}

	return rows, nil
}

// swapPositions atomically exchanges the positions of two live pages —
// a single UPDATE so a mid-failure can't leave a half-swapped order.
func (s *Store) swapPositions(ctx context.Context, idA, idB pgtype.UUID, posA, posB int32) error {
	return s.write.SwapDocPagePositions(ctx, gen.SwapDocPagePositionsParams{
		IDA:       idA,
		IDB:       idB,
		PositionA: posA,
		PositionB: posB,
	})
}

// listAll returns all doc pages (any audience) for admin listing, optionally
// filtered to one review_status ("" = any status). showDeleted swaps the
// live-page view for the trash view (Rule 11).
func (s *Store) listAll(ctx context.Context, status string, showDeleted bool, limit, offset int32) ([]gen.ListAllDocPagesRow, error) {
	rows, err := s.read.ListAllDocPages(ctx, gen.ListAllDocPagesParams{
		StatusFilter: status,
		ShowDeleted:  showDeleted,
		ResultLimit:  limit,
		ResultOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing all doc pages: %w", err)
	}

	return rows, nil
}

// listPublished returns all published doc pages across audiences.
func (s *Store) listPublished(ctx context.Context) ([]gen.ListPublishedDocPagesRow, error) {
	rows, err := s.read.ListPublishedDocPages(ctx, gen.ListPublishedDocPagesParams{ResultLimit: 0, ResultOffset: 0})
	if err != nil {
		return nil, fmt.Errorf("listing published doc pages: %w", err)
	}

	return rows, nil
}

// likeEscaper escapes LIKE metacharacters so a literal "%" or "_" in the
// search term isn't treated as a wildcard (Postgres' default LIKE escape is
// the backslash).
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// search returns published doc pages matching the query by title or content.
// When audience is non-nil, results are filtered to that audience section.
func (s *Store) search(ctx context.Context, query string, audience *string, limit int32) ([]gen.SearchDocPagesRow, error) {
	rows, err := s.read.SearchDocPages(ctx, gen.SearchDocPagesParams{
		Audience:    audience,
		Query:       likeEscaper.Replace(query),
		ResultLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching doc pages: %w", err)
	}

	return rows, nil
}

// updateStatus updates only the review status and reviewer of a doc page.
func (s *Store) updateStatus(ctx context.Context, params gen.UpdateDocPageStatusParams) error {
	if err := s.write.UpdateDocPageStatus(ctx, params); err != nil {
		return fmt.Errorf("updating doc page status: %w", err)
	}

	return nil
}

// getStatus returns the status row (review_status, created_by, ai_generated)
// driving transition checks.
func (s *Store) getStatus(ctx context.Context, id pgtype.UUID) (gen.GetDocPageStatusRow, error) {
	row, err := s.read.GetDocPageStatus(ctx, id)
	if err != nil {
		return gen.GetDocPageStatusRow{}, fmt.Errorf("getting doc page status: %w", err)
	}

	return row, nil
}

// softDelete marks a doc page deleted (Rule 11). parent_id is left intact —
// the nav tree hides the row while trashed and restore brings the whole
// subtree back with its hierarchy. Returns rows affected; 0 means the page
// was already trashed or doesn't exist.
func (s *Store) softDelete(ctx context.Context, id pgtype.UUID) (int64, error) {
	rows, err := s.write.SoftDeleteDocPage(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("soft-deleting doc page: %w", err)
	}

	return rows, nil
}

// restore clears deleted_at on a trashed page. Returns rows affected; 0
// means the page wasn't in the trash.
func (s *Store) restore(ctx context.Context, id pgtype.UUID) (int64, error) {
	rows, err := s.write.RestoreDocPage(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("restoring doc page: %w", err)
	}

	return rows, nil
}

// hardDelete permanently removes a doc page and orphans its children in a
// single statement (HardDeleteDocPage): parent_id is a real FK, so unlinking
// has to ride along with the delete — the FK check at statement end sees the
// already-unlinked children. The statement itself only matches
// deleted_at IS NOT NULL — Rule 11's soft-delete-first gate at the SQL
// layer; 0 rows also closes the race where a page is restored between the
// caller's check and this delete.
func (s *Store) hardDelete(ctx context.Context, id pgtype.UUID) error {
	rows, err := s.write.HardDeleteDocPage(ctx, id)
	if err != nil {
		return fmt.Errorf("deleting doc page: %w", err)
	}
	if rows == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// getFeedback returns aggregate helpful counters plus the user's recorded
// vote for a published page (§44 amendment 2). When write is true the query
// runs on the writer pool — used after a vote so replica lag can't return
// pre-vote counts in the POST response.
func (s *Store) getFeedback(ctx context.Context, audience, slug string, userID pgtype.UUID, write bool) (gen.GetDocPageFeedbackRow, error) {
	q := s.read
	if write {
		q = s.write
	}

	row, err := q.GetDocPageFeedback(ctx, gen.GetDocPageFeedbackParams{
		UserID:   userID,
		Audience: audience,
		Slug:     slug,
	})
	if err != nil {
		return gen.GetDocPageFeedbackRow{}, fmt.Errorf("getting doc page feedback: %w", err)
	}

	return row, nil
}

// setVote atomically upserts the user's vote and moves the aggregate
// counters (§44 amendment 2). Returns rows affected by the counter update —
// 0 means the page ID did not match a doc_pages row.
func (s *Store) setVote(ctx context.Context, pageID, userID pgtype.UUID, vote string) (int64, error) {
	rows, err := s.write.SetDocPageVote(ctx, gen.SetDocPageVoteParams{
		PageID: pageID,
		UserID: userID,
		Vote:   vote,
	})
	if err != nil {
		return 0, fmt.Errorf("recording doc page vote: %w", err)
	}

	return rows, nil
}

// deleteVote atomically removes the user's vote and moves the aggregate
// counters down (§44 amendment 2 — re-clicking the same vote retracts it,
// same toggle as wiki page feedback).
func (s *Store) deleteVote(ctx context.Context, pageID, userID pgtype.UUID) error {
	return s.write.DeleteDocPageVote(ctx, gen.DeleteDocPageVoteParams{
		PageID: pageID,
		UserID: userID,
	})
}

// searchInAudiences runs the single-query multi-audience fallback search
// (§44 amendment 2).
func (s *Store) searchInAudiences(ctx context.Context, query string, audiences []string, limit int32) ([]gen.SearchDocPagesInAudiencesRow, error) {
	rows, err := s.read.SearchDocPagesInAudiences(ctx, gen.SearchDocPagesInAudiencesParams{
		Audiences:   audiences,
		Query:       likeEscaper.Replace(query),
		ResultLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("searching doc pages across audiences: %w", err)
	}

	return rows, nil
}
