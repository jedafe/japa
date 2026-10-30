package docs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// mapWriteErr translates Postgres constraint violations into sentinel errors
// so handlers emit a 409 instead of a raw 500 (§44 amendment 2).
func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: %s", errpkg.ErrDuplicate, pgErr.ConstraintName)
	}

	return err
}

// Store provides low-level database access for documentation pages.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// getBySlug returns a published doc page by audience and slug.
func (s *Store) getBySlug(ctx context.Context, audience, slug string) (gen.DocPage, error) {
	row, err := s.read.GetDocPageBySlug(ctx, gen.GetDocPageBySlugParams{
		Audience: audience,
		Slug:     slug,
	})
	if err != nil {
		return gen.DocPage{}, fmt.Errorf("getting doc page by slug: %w", err)
	}

	return gen.DocPage{
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
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

// getBySlugAnyStatus returns a doc page by audience+slug regardless of
// review status. Editor preview only — callers must gate on docs.manage
// (§44 amendment 2).
func (s *Store) getBySlugAnyStatus(ctx context.Context, audience, slug string) (gen.DocPage, error) {
	row, err := s.read.GetDocPageBySlugAnyStatus(ctx, gen.GetDocPageBySlugAnyStatusParams{
		Audience: audience,
		Slug:     slug,
	})
	if err != nil {
		return gen.DocPage{}, fmt.Errorf("getting doc page by slug (any status): %w", err)
	}

	return gen.DocPage{
		ID:             row.ID,
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
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

// getByID returns a doc page by ID.
func (s *Store) getByID(ctx context.Context, id pgtype.UUID) (gen.DocPage, error) {
	row, err := s.read.GetDocPageByID(ctx,
		id,
	)
	if err != nil {
		return gen.DocPage{}, fmt.Errorf("getting doc page by ID: %w", err)
	}

	return gen.DocPage{
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
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}, nil
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

// listAllByAudience returns all doc pages (any status) for an audience.
func (s *Store) listAllByAudience(ctx context.Context, audience string) ([]gen.ListAllDocPagesByAudienceRow, error) {
	rows, err := s.read.ListAllDocPagesByAudience(ctx,

		audience,
	)
	if err != nil {
		return nil, fmt.Errorf("listing all doc pages by audience: %w", err)
	}

	return rows, nil
}

// listAll returns all doc pages (any status, any audience) for admin listing.
func (s *Store) listAll(ctx context.Context) ([]gen.ListAllDocPagesRow, error) {
	rows, err := s.read.ListAllDocPages(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing all doc pages: %w", err)
	}

	return rows, nil
}

// listPublished returns all published doc pages across audiences.
func (s *Store) listPublished(ctx context.Context) ([]gen.ListPublishedDocPagesRow, error) {
	rows, err := s.read.ListPublishedDocPages(ctx)
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

// create inserts a new doc page and returns its ID.
func (s *Store) create(ctx context.Context, params gen.CreateDocPageParams) (pgtype.UUID, error) {
	id, err := s.write.CreateDocPage(ctx, params)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creating doc page: %w", mapWriteErr(err))
	}

	return id, nil
}

// update modifies an existing doc page. Returns the number of rows written —
// 0 means the optimistic-lock guard or the id filter rejected the write.
func (s *Store) update(ctx context.Context, params gen.UpdateDocPageParams) (int64, error) {
	rows, err := s.write.UpdateDocPage(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("updating doc page: %w", mapWriteErr(err))
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

// getStatus returns the current review_status of a doc page.
func (s *Store) getStatus(ctx context.Context, id pgtype.UUID) (string, error) {
	status, err := s.read.GetDocPageStatus(ctx, id)
	if err != nil {
		return "", fmt.Errorf("getting doc page status: %w", err)
	}

	return status, nil
}

// delete removes a doc page and orphans its children. parent_id is an FK to
// doc_pages, so children must be unlinked before the delete or the FK would
// reject it; the nav tree promotes orphans to roots.
func (s *Store) delete(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.ClearDocPageParent(ctx, id); err != nil {
		return fmt.Errorf("unlinking doc page children: %w", err)
	}

	if err := s.write.DeleteDocPage(ctx,
		id,
	); err != nil {
		return fmt.Errorf("deleting doc page: %w", err)
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
