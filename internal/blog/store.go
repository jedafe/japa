package blog

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for blog operations. The Service
// layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
	pool  *pgxpool.Pool
}

// NewStore creates a Store backed by the given gen.Queries instances and pool.
func NewStore(read, write *gen.Queries, pool *pgxpool.Pool) *Store {
	return &Store{read: read, write: write, pool: pool}
}

// ---------------------------------------------------------------------------
// Blog Series
// ---------------------------------------------------------------------------

func (s *Store) createBlogSeries(ctx context.Context, p gen.CreateBlogSeriesParams) (gen.BlogSeries, error) {
	row, err := s.write.CreateBlogSeries(ctx, p)
	if err != nil {
		return gen.BlogSeries{}, fmt.Errorf("creating blog series: %w", err)
	}

	return row, nil
}

func (s *Store) getBlogSeriesByID(ctx context.Context, id pgtype.UUID) (gen.BlogSeries, error) {
	row, err := s.read.GetBlogSeriesByID(ctx, id)
	if err != nil {
		return gen.BlogSeries{}, fmt.Errorf("getting blog series by ID: %w", err)
	}

	return row, nil
}

func (s *Store) getBlogSeriesBySlug(ctx context.Context, slug string) (gen.GetBlogSeriesBySlugRow, error) {
	row, err := s.read.GetBlogSeriesBySlug(ctx, slug)
	if err != nil {
		return gen.GetBlogSeriesBySlugRow{}, fmt.Errorf("getting blog series by slug: %w", err)
	}

	return row, nil
}

func (s *Store) listBlogSeriesByTenant(ctx context.Context, q, contentType string, limit, offset int32) ([]gen.ListBlogSeriesByTenantRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListBlogSeriesByTenant(ctx, gen.ListBlogSeriesByTenantParams{

		Q:           q,
		ContentType: contentType,
		PageLimit:   limit,
		PageOffset:  offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing blog series by tenant: %w", err)
	}

	return rows, nil
}

func (s *Store) listBlogSeriesByAuthor(ctx context.Context, authorUserID pgtype.UUID, limit, offset int32) ([]gen.ListBlogSeriesByAuthorRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListBlogSeriesByAuthor(ctx, gen.ListBlogSeriesByAuthorParams{

		AuthorUserID: authorUserID,
		PageLimit:    limit,
		PageOffset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing blog series by author: %w", err)
	}

	return rows, nil
}

func (s *Store) countBlogSeriesByTenant(ctx context.Context, q, contentType string) (int64, error) {
	count, err := s.read.CountBlogSeriesByTenant(ctx, gen.CountBlogSeriesByTenantParams{

		Q:           q,
		ContentType: contentType,
	})
	if err != nil {
		return 0, fmt.Errorf("counting blog series by tenant: %w", err)
	}

	return count, nil
}

func (s *Store) countBlogSeriesByAuthor(ctx context.Context, authorUserID pgtype.UUID) (int64, error) {
	count, err := s.read.CountBlogSeriesByAuthor(ctx,

		authorUserID,
	)
	if err != nil {
		return 0, fmt.Errorf("counting blog series by author: %w", err)
	}

	return count, nil
}

func (s *Store) updateBlogSeries(ctx context.Context, p gen.UpdateBlogSeriesParams) (gen.BlogSeries, error) {
	row, err := s.write.UpdateBlogSeries(ctx, p)
	if err != nil {
		return gen.BlogSeries{}, fmt.Errorf("updating blog series: %w", err)
	}

	return row, nil
}

func (s *Store) deleteBlogSeries(ctx context.Context, id pgtype.UUID) error {
	if err := s.write.DeleteBlogSeries(ctx, id); err != nil {
		return fmt.Errorf("deleting blog series: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Blog Series Posts
// ---------------------------------------------------------------------------

func (s *Store) addPostToSeries(ctx context.Context, p gen.AddPostToSeriesParams) error {
	if err := s.write.AddPostToSeries(ctx, p); err != nil {
		return fmt.Errorf("adding post to series: %w", err)
	}

	return nil
}

func (s *Store) removePostFromSeries(ctx context.Context, seriesID, contentID pgtype.UUID) error {
	if err := s.write.RemovePostFromSeries(ctx, gen.RemovePostFromSeriesParams{
		SeriesID:  seriesID,
		ContentID: contentID,
	}); err != nil {
		return fmt.Errorf("removing post from series: %w", err)
	}

	return nil
}

func (s *Store) listPostsInSeries(ctx context.Context, seriesID pgtype.UUID) ([]gen.ListPostsInSeriesRow, error) {
	rows, err := s.read.ListPostsInSeries(ctx,
		seriesID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing posts in series: %w", err)
	}

	return rows, nil
}

// getContentTenantID is a no-op compatibility helper. Cross-tenant verification
// is no longer required in the single-tenant deployment.
func (s *Store) getContentTenantID(ctx context.Context, contentID pgtype.UUID) (pgtype.UUID, error) {
	_ = contentID
	return pgtype.UUID{}, nil
}

// getSeriesForContent returns the series_id for a given content_id, or
// pg.ErrNoRows if the content is not part of any series. Used by the editor
// load to restore the series selector when editing existing content.
// BUG-009: tenant-scoped to prevent cross-tenant information disclosure.
func (s *Store) getSeriesForContent(ctx context.Context, contentID pgtype.UUID) (pgtype.UUID, error) {
	return s.read.GetSeriesForContent(ctx,
		contentID,
	)
}

func (s *Store) reorderSeriesPosts(ctx context.Context, seriesID, contentID pgtype.UUID, position int32) error {
	if err := s.write.ReorderSeriesPosts(ctx, gen.ReorderSeriesPostsParams{
		SeriesID:  seriesID,
		ContentID: contentID,
		Position:  position,
	}); err != nil {
		return fmt.Errorf("reordering series posts: %w", err)
	}

	return nil
}

// bulkReorderSeriesPosts replaces all posts in a series with the given
// ordered list inside a single transaction, eliminating the N+1 pattern.
func (s *Store) bulkReorderSeriesPosts(ctx context.Context, seriesID pgtype.UUID, posts []struct {
	ContentID pgtype.UUID
	Position  int32
},
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning bulk reorder transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	q := s.write.WithTx(tx)

	if err := q.DeleteSeriesPosts(ctx, seriesID); err != nil {
		return fmt.Errorf("deleting series posts for reorder: %w", err)
	}

	for _, p := range posts {
		if err := q.InsertSeriesPostWithPosition(ctx, gen.InsertSeriesPostWithPositionParams{
			SeriesID:  seriesID,
			ContentID: p.ContentID,
			Position:  p.Position,
		}); err != nil {
			return fmt.Errorf("inserting post during bulk reorder: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Content Reviews
// ---------------------------------------------------------------------------

// contentReviewFromRow converts any of the sqlc-generated *Row types for
// content_reviews into a gen.ContentReview. The Row types all share the same
// fields as ContentReview minus Source (which no caller uses).
func contentReviewFromRow(r any) gen.ContentReview {
	switch v := r.(type) {
	case gen.CreateContentReviewRow:
		return gen.ContentReview{ID: v.ID, ContentID: v.ContentID, ReviewerUserID: v.ReviewerUserID, Status: v.Status, Feedback: v.Feedback, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, DeletedAt: v.DeletedAt}
	case gen.GetContentReviewByIDRow:
		return gen.ContentReview{ID: v.ID, ContentID: v.ContentID, ReviewerUserID: v.ReviewerUserID, Status: v.Status, Feedback: v.Feedback, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, DeletedAt: v.DeletedAt}
	case gen.ListContentReviewsByContentRow:
		return gen.ContentReview{ID: v.ID, ContentID: v.ContentID, ReviewerUserID: v.ReviewerUserID, Status: v.Status, Feedback: v.Feedback, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, DeletedAt: v.DeletedAt}
	case gen.ListPendingContentReviewsRow:
		return gen.ContentReview{ID: v.ID, ContentID: v.ContentID, ReviewerUserID: v.ReviewerUserID, Status: v.Status, Feedback: v.Feedback, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, DeletedAt: v.DeletedAt}
	case gen.UpdateContentReviewStatusRow:
		return gen.ContentReview{ID: v.ID, ContentID: v.ContentID, ReviewerUserID: v.ReviewerUserID, Status: v.Status, Feedback: v.Feedback, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, DeletedAt: v.DeletedAt}
	}

	return gen.ContentReview{}
}

func (s *Store) createContentReview(ctx context.Context, p gen.CreateContentReviewParams) (gen.ContentReview, error) {
	row, err := s.write.CreateContentReview(ctx, p)
	if err != nil {
		return gen.ContentReview{}, fmt.Errorf("creating content review: %w", err)
	}

	return contentReviewFromRow(row), nil
}

func (s *Store) getContentReviewByID(ctx context.Context, id pgtype.UUID) (gen.ContentReview, error) {
	row, err := s.read.GetContentReviewByID(ctx, id)
	if err != nil {
		return gen.ContentReview{}, fmt.Errorf("getting content review by ID: %w", err)
	}

	return contentReviewFromRow(row), nil
}

func (s *Store) listContentReviewsByContent(ctx context.Context, contentID pgtype.UUID) ([]gen.ContentReview, error) {
	rows, err := s.read.ListContentReviewsByContent(ctx,
		contentID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing content reviews by content: %w", err)
	}

	out := make([]gen.ContentReview, len(rows))
	for i, r := range rows {
		out[i] = contentReviewFromRow(r)
	}

	return out, nil
}

func (s *Store) listPendingContentReviews(ctx context.Context, limit, offset int32) ([]gen.ContentReview, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPendingContentReviews(ctx, gen.ListPendingContentReviewsParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing pending content reviews: %w", err)
	}

	out := make([]gen.ContentReview, len(rows))
	for i, r := range rows {
		out[i] = contentReviewFromRow(r)
	}

	return out, nil
}

func (s *Store) updateContentReviewStatus(ctx context.Context, p gen.UpdateContentReviewStatusParams) (gen.ContentReview, error) {
	row, err := s.write.UpdateContentReviewStatus(ctx, p)
	if err != nil {
		return gen.ContentReview{}, fmt.Errorf("updating content review status: %w", err)
	}

	return contentReviewFromRow(row), nil
}

func (s *Store) deleteContentReview(ctx context.Context, id pgtype.UUID) error {
	return s.write.DeleteContentReview(ctx,
		id,
	)
}

// ---------------------------------------------------------------------------
// Content Featured
// ---------------------------------------------------------------------------

func (s *Store) featureContent(ctx context.Context, p gen.FeatureContentParams) (gen.ContentFeatured, error) {
	row, err := s.write.FeatureContent(ctx, p)
	if err != nil {
		return gen.ContentFeatured{}, fmt.Errorf("featuring content: %w", err)
	}

	return row, nil
}

func (s *Store) unfeatureContent(ctx context.Context, contentID pgtype.UUID) error {
	if err := s.write.UnfeatureContent(ctx,

		contentID,
	); err != nil {
		return fmt.Errorf("unfeaturing content: %w", err)
	}

	return nil
}

func (s *Store) listFeaturedPosts(ctx context.Context, limit, offset int32) ([]gen.ContentFeatured, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListFeaturedPosts(ctx, gen.ListFeaturedPostsParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing featured posts: %w", err)
	}

	return rows, nil
}

func (s *Store) updateFeaturedOrder(ctx context.Context, id pgtype.UUID, featuredOrder int32) error {
	if err := s.write.UpdateFeaturedOrder(ctx, gen.UpdateFeaturedOrderParams{
		ID: id,

		FeaturedOrder: featuredOrder,
	}); err != nil {
		return fmt.Errorf("updating featured order: %w", err)
	}

	return nil
}

func (s *Store) expireFeaturedPosts(ctx context.Context) error {
	if err := s.write.ExpireFeaturedPosts(ctx); err != nil {
		return fmt.Errorf("expiring featured posts: %w", err)
	}

	return nil
}

// listFeaturedBlogPostCards returns enriched featured post cards with JOINs
// to contents, users, and media (H6).
func (s *Store) listFeaturedBlogPostCards(ctx context.Context, limit, offset int32) ([]gen.ListFeaturedBlogPostCardsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListFeaturedBlogPostCards(ctx, gen.ListFeaturedBlogPostCardsParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing featured blog post cards: %w", err)
	}

	return rows, nil
}

// countFeaturedBlogPostCards returns the total number of active featured blog
// posts for pagination meta.
func (s *Store) countFeaturedBlogPostCards(ctx context.Context) (int64, error) {
	count, err := s.read.CountFeaturedBlogPostCards(ctx)
	if err != nil {
		return 0, fmt.Errorf("counting featured blog post cards: %w", err)
	}

	return count, nil
}

// bulkUpdateFeaturedOrder updates all featured post orders in a single
// transaction (M3 — all-or-nothing on failure).
func (s *Store) bulkUpdateFeaturedOrder(ctx context.Context, items []struct {
	ID    pgtype.UUID
	Order int32
},
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning bulk featured reorder transaction: %w", err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	q := s.write.WithTx(tx)

	for _, item := range items {
		if err := q.UpdateFeaturedOrder(ctx, gen.UpdateFeaturedOrderParams{
			ID: item.ID,

			FeaturedOrder: item.Order,
		}); err != nil {
			return fmt.Errorf("updating featured order in bulk: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Blog Post Detail / List (enriched DTOs)
// ---------------------------------------------------------------------------

// getBlogPostDetail returns a single blog post with author, avatar, featured
// image, space name, tags, and series navigation (C4).
// Authors can view their own drafts; super-admins can view any post in the
// tenant for moderation. Ref: japaconnect-engineering/sections/24-blog-extension.md.
func (s *Store) getBlogPostDetail(ctx context.Context, slug string, viewerUserID pgtype.UUID, isSuperAdmin bool) (gen.GetBlogPostDetailRow, error) {
	row, err := s.read.GetBlogPostDetail(ctx, gen.GetBlogPostDetailParams{
		Slug: slug,

		ViewerUserID: viewerUserID,
		IsSuperAdmin: &isSuperAdmin,
	})
	if err != nil {
		return gen.GetBlogPostDetailRow{}, fmt.Errorf("getting blog post detail: %w", err)
	}

	return row, nil
}

// listPublishedBlogPosts returns published blog post cards with author and
// featured image, status-filtered in SQL (C5/H3).
// BUG-005: offset-based pagination per Rule 5 (page_size + page_offset).
// BUG-011: accepts §24-mandated filters (space_id, tag, series_id, sort,
// author_id) via BlogListFilter. All fields zero/nil = no filter.
func (s *Store) listPublishedBlogPosts(ctx context.Context, f BlogListFilter, limit, offset int32) ([]gen.ListPublishedBlogPostsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	sort := f.Sort
	if sort == "" {
		sort = "latest"
	}

	rows, err := s.read.ListPublishedBlogPosts(ctx, gen.ListPublishedBlogPostsParams{

		SpaceID:    f.SpaceID,
		AuthorID:   f.AuthorID,
		SeriesID:   f.SeriesID,
		Tag:        f.Tag,
		Sort:       sort,
		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing published blog posts: %w", err)
	}

	return rows, nil
}

// countPublishedBlogPosts returns the total number of published blog posts
// for pagination meta.
// BUG-011: accepts the same §24 filters as listPublishedBlogPosts so the
// pagination total reflects the active filter set.
func (s *Store) countPublishedBlogPosts(ctx context.Context, f BlogListFilter) (int64, error) {
	count, err := s.read.CountPublishedBlogPosts(ctx, gen.CountPublishedBlogPostsParams{

		SpaceID:  f.SpaceID,
		AuthorID: f.AuthorID,
		SeriesID: f.SeriesID,
		Tag:      f.Tag,
	})
	if err != nil {
		return 0, fmt.Errorf("counting published blog posts: %w", err)
	}

	return count, nil
}

// listAdminBlogPosts returns all-status blog post cards with author name
// for the admin /blog table (BUG-B). Offset-paginated per Rule 5.
func (s *Store) listAdminBlogPosts(ctx context.Context, limit, offset int32) ([]gen.ListAdminBlogPostsRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	rows, err := s.read.ListAdminBlogPosts(ctx, gen.ListAdminBlogPostsParams{

		PageOffset: offset,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing admin blog posts: %w", err)
	}

	return rows, nil
}

// listPendingReviewSummaries returns joined review-queue rows with post
// title/slug/author/excerpt for the admin review-queue (C3).
func (s *Store) listPendingReviewSummaries(ctx context.Context, limit, offset int32) ([]gen.ListPendingReviewSummariesRow, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPendingReviewSummaries(ctx, gen.ListPendingReviewSummariesParams{

		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing pending review summaries: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// RSS Feed
// ---------------------------------------------------------------------------

func (s *Store) listPublishedPostsForRSS(ctx context.Context, limit int32) ([]gen.ListPublishedPostsForRSSRow, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.read.ListPublishedPostsForRSS(ctx,

		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing published posts for RSS: %w", err)
	}

	return rows, nil
}
