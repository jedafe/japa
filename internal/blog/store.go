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

func (s *Store) getBlogSeriesByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.BlogSeries, error) {
	row, err := s.read.GetBlogSeriesByID(ctx, gen.GetBlogSeriesByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.BlogSeries{}, fmt.Errorf("getting blog series by ID: %w", err)
	}

	return row, nil
}

func (s *Store) getBlogSeriesBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.BlogSeries, error) {
	row, err := s.read.GetBlogSeriesBySlug(ctx, gen.GetBlogSeriesBySlugParams{Slug: slug, TenantID: tenantID})
	if err != nil {
		return gen.BlogSeries{}, fmt.Errorf("getting blog series by slug: %w", err)
	}

	return row, nil
}

func (s *Store) listBlogSeriesByTenant(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.BlogSeries, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListBlogSeriesByTenant(ctx, gen.ListBlogSeriesByTenantParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing blog series by tenant: %w", err)
	}

	return rows, nil
}

func (s *Store) listBlogSeriesByAuthor(ctx context.Context, tenantID, authorUserID pgtype.UUID, limit, offset int32) ([]gen.BlogSeries, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListBlogSeriesByAuthor(ctx, gen.ListBlogSeriesByAuthorParams{
		TenantID:     tenantID,
		AuthorUserID: authorUserID,
		PageLimit:    limit,
		PageOffset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing blog series by author: %w", err)
	}

	return rows, nil
}

func (s *Store) updateBlogSeries(ctx context.Context, p gen.UpdateBlogSeriesParams) (gen.BlogSeries, error) {
	row, err := s.write.UpdateBlogSeries(ctx, p)
	if err != nil {
		return gen.BlogSeries{}, fmt.Errorf("updating blog series: %w", err)
	}

	return row, nil
}

func (s *Store) deleteBlogSeries(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteBlogSeries(ctx, gen.DeleteBlogSeriesParams{ID: id, TenantID: tenantID}); err != nil {
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
	rows, err := s.read.ListPostsInSeries(ctx, seriesID)
	if err != nil {
		return nil, fmt.Errorf("listing posts in series: %w", err)
	}

	return rows, nil
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
		if err := q.AddPostToSeries(ctx, gen.AddPostToSeriesParams{
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

func (s *Store) createContentReview(ctx context.Context, p gen.CreateContentReviewParams) (gen.ContentReview, error) {
	row, err := s.write.CreateContentReview(ctx, p)
	if err != nil {
		return gen.ContentReview{}, fmt.Errorf("creating content review: %w", err)
	}

	return row, nil
}

func (s *Store) getContentReviewByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.ContentReview, error) {
	row, err := s.read.GetContentReviewByID(ctx, gen.GetContentReviewByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.ContentReview{}, fmt.Errorf("getting content review by ID: %w", err)
	}

	return row, nil
}

func (s *Store) listContentReviewsByContent(ctx context.Context, contentID, tenantID pgtype.UUID) ([]gen.ContentReview, error) {
	rows, err := s.read.ListContentReviewsByContent(ctx, gen.ListContentReviewsByContentParams{
		ContentID: contentID,
		TenantID:  tenantID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing content reviews by content: %w", err)
	}

	return rows, nil
}

func (s *Store) listPendingContentReviews(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ContentReview, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListPendingContentReviews(ctx, gen.ListPendingContentReviewsParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing pending content reviews: %w", err)
	}

	return rows, nil
}

func (s *Store) updateContentReviewStatus(ctx context.Context, p gen.UpdateContentReviewStatusParams) (gen.ContentReview, error) {
	row, err := s.write.UpdateContentReviewStatus(ctx, p)
	if err != nil {
		return gen.ContentReview{}, fmt.Errorf("updating content review status: %w", err)
	}

	return row, nil
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

func (s *Store) unfeatureContent(ctx context.Context, tenantID, contentID pgtype.UUID) error {
	if err := s.write.UnfeatureContent(ctx, gen.UnfeatureContentParams{
		TenantID:  tenantID,
		ContentID: contentID,
	}); err != nil {
		return fmt.Errorf("unfeaturing content: %w", err)
	}

	return nil
}

func (s *Store) listFeaturedPosts(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ContentFeatured, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListFeaturedPosts(ctx, gen.ListFeaturedPostsParams{
		TenantID:   tenantID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("listing featured posts: %w", err)
	}

	return rows, nil
}

func (s *Store) updateFeaturedOrder(ctx context.Context, id, tenantID pgtype.UUID, featuredOrder int32) error {
	if err := s.write.UpdateFeaturedOrder(ctx, gen.UpdateFeaturedOrderParams{
		ID:            id,
		TenantID:      tenantID,
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

// ---------------------------------------------------------------------------
// RSS Feed
// ---------------------------------------------------------------------------

func (s *Store) listPublishedPostsForRSS(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]gen.ListPublishedPostsForRSSRow, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.read.ListPublishedPostsForRSS(ctx, gen.ListPublishedPostsForRSSParams{
		TenantID:  tenantID,
		PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("listing published posts for RSS: %w", err)
	}

	return rows, nil
}
