package blog

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// CreateSeriesInput holds parameters for creating a new blog series.
type CreateSeriesInput struct {
	Title        string
	Slug         string
	Description  string
	CoverImageID pgtype.UUID
}

// CreateSeries creates a new blog series.
func (s *Service) CreateSeries(ctx context.Context, authorUserID pgtype.UUID, input CreateSeriesInput) (gen.BlogSeries, error) {
	if input.Title == "" {
		return gen.BlogSeries{}, errors.New("title is required")
	}

	if input.Slug == "" {
		return gen.BlogSeries{}, errors.New("slug is required")
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	return s.store.createBlogSeries(ctx, gen.CreateBlogSeriesParams{
		Title:        input.Title,
		Slug:         input.Slug,
		Description:  description,
		CoverImageID: input.CoverImageID,
		AuthorUserID: authorUserID,
	})
}

// GetSeries retrieves a blog series by ID.
func (s *Service) GetSeries(ctx context.Context, id pgtype.UUID) (gen.BlogSeries, error) {
	return s.store.getBlogSeriesByID(ctx, id)
}

// GetSeriesBySlug retrieves a blog series by slug and returns a public DTO
// with a computed cover_image_url.
func (s *Service) GetSeriesBySlug(ctx context.Context, slug string) (Series, error) {
	row, err := s.store.getBlogSeriesBySlug(ctx, slug)
	if err != nil {
		return Series{}, err
	}

	return seriesFromRow(row), nil
}

// ListSeriesByPlatform returns paginated public series DTOs for a platform,
// optionally filtered by the autocomplete query `q` (L2 — filter now in SQL).
// sort: "latest" (default), "oldest", or "title".
func (s *Service) ListSeriesByPlatform(ctx context.Context, q, contentType, sort string, limit, offset int32) ([]Series, error) {
	rows, err := s.store.listBlogSeriesByPlatform(ctx, q, contentType, sort, limit, offset)
	if err != nil {
		return nil, err
	}

	out := make([]Series, len(rows))
	for i, r := range rows {
		out[i] = seriesFromRow(r)
	}

	return out, nil
}

// CountSeriesByPlatform returns the total number of platform-scoped series,
// respecting the same `q` filter as ListSeriesByPlatform.
func (s *Service) CountSeriesByPlatform(ctx context.Context, q, contentType string) (int64, error) {
	return s.store.countBlogSeriesByPlatform(ctx, q, contentType)
}

// ListSeriesByAuthor returns paginated public series DTOs for an author.
// Shares the q / content_type / sort filters with ListSeriesByPlatform so
// the "My series" tab offers the same toolbar as the directory.
func (s *Service) ListSeriesByAuthor(ctx context.Context, authorUserID pgtype.UUID, q, contentType, sort string, limit, offset int32) ([]Series, error) {
	rows, err := s.store.listBlogSeriesByAuthor(ctx, authorUserID, q, contentType, sort, limit, offset)
	if err != nil {
		return nil, err
	}

	out := make([]Series, len(rows))
	for i, r := range rows {
		out[i] = seriesFromRow(r)
	}

	return out, nil
}

// CountSeriesByAuthor returns the total number of blog series for an author,
// respecting the same q / content_type filters as ListSeriesByAuthor.
func (s *Service) CountSeriesByAuthor(ctx context.Context, authorUserID pgtype.UUID, q, contentType string) (int64, error) {
	return s.store.countBlogSeriesByAuthor(ctx, authorUserID, q, contentType)
}

// UpdateSeries updates a blog series. M2: slug is now updatable.
func (s *Service) UpdateSeries(ctx context.Context, id pgtype.UUID, title, slug, description string, coverImageID pgtype.UUID, status string) (gen.BlogSeries, error) {
	if title == "" {
		return gen.BlogSeries{}, errors.New("title is required")
	}

	if status == "" {
		status = SeriesStatusActive
	}

	var desc *string
	if description != "" {
		desc = &description
	}

	var slugPtr *string
	if slug != "" {
		slugPtr = &slug
	}

	return s.store.updateBlogSeries(ctx, gen.UpdateBlogSeriesParams{
		ID: id,

		Title:        title,
		Slug:         slugPtr,
		Description:  desc,
		CoverImageID: coverImageID,
		Status:       status,
	})
}

// DeleteSeries deletes a blog series. force=true permanently removes an
// already-soft-deleted row (ErrNotDeleted while live).
func (s *Service) DeleteSeries(ctx context.Context, id pgtype.UUID, force bool) error {
	if force {
		return s.store.hardDeleteBlogSeries(ctx, id)
	}

	return s.store.deleteBlogSeries(ctx, id)
}

// ---------------------------------------------------------------------------
// Series Posts
// ---------------------------------------------------------------------------

// AddPostToSeries adds a post to a series, auto-appending at the next
// available position. H7: verifies the content belongs to the same platform
// before linking to prevent cross-platform content linkage. The position is
// computed server-side (MAX+1) to avoid colliding with the unique constraint
// on (series_id, position); use ReorderPostsInSeries for reordering.
func (s *Service) AddPostToSeries(ctx context.Context, seriesID, contentID pgtype.UUID, position int32) error {
	return s.store.addPostToSeries(ctx, gen.AddPostToSeriesParams{
		SeriesID:  seriesID,
		ContentID: contentID,
	})
}

// RemovePostFromSeries removes a post from a series.
// BUG-003: verifies the series belongs to the caller's platform before
// deleting, otherwise an attacker with blog.series.edit in platform A could
// remove posts from a series in platform B by knowing its series_id.
func (s *Service) RemovePostFromSeries(ctx context.Context, seriesID, contentID pgtype.UUID) error {
	if _, err := s.store.getBlogSeriesByID(ctx, seriesID); err != nil {
		return fmt.Errorf("verifying series platform: %w", err)
	}

	return s.store.removePostFromSeries(ctx, seriesID, contentID)
}

// ListPostsInSeries returns all posts in a series ordered by position.
// H5: now platform-scoped and filters soft-deleted contents.
func (s *Service) ListPostsInSeries(ctx context.Context, seriesID pgtype.UUID) ([]gen.ListPostsInSeriesRow, error) {
	return s.store.listPostsInSeries(ctx, seriesID)
}

// GetSeriesForContent returns the series_id for a given content_id, or
// pgx.ErrNoRows if the content is not part of any series. Used by the
// editor load to restore the series selector. BUG-009: platform-scoped.
func (s *Service) GetSeriesForContent(ctx context.Context, contentID pgtype.UUID) (pgtype.UUID, error) {
	return s.store.getSeriesForContent(ctx, contentID)
}

// ReorderPostsInSeries updates the position of a post within a series.
// BUG-004: verifies the series belongs to the caller's platform before
// mutating (same cross-platform risk as BulkReorderSeriesPosts).
func (s *Service) ReorderPostsInSeries(ctx context.Context, seriesID, contentID pgtype.UUID, position int32) error {
	if _, err := s.store.getBlogSeriesByID(ctx, seriesID); err != nil {
		return fmt.Errorf("verifying series platform: %w", err)
	}

	return s.store.reorderSeriesPosts(ctx, seriesID, contentID, position)
}

// BulkReorderSeriesPosts atomically replaces all posts in a series with the
// given ordered list, eliminating the N+1 round-trip of per-post updates.
// BUG-004: verifies the series belongs to the caller's platform before
// mutating, otherwise an attacker with blog.series.edit in platform A could
// wipe and rewrite the post order of a series in platform B.
func (s *Service) BulkReorderSeriesPosts(ctx context.Context, seriesID pgtype.UUID, posts []struct {
	ContentID pgtype.UUID
	Position  int32
},
) error {
	if _, err := s.store.getBlogSeriesByID(ctx, seriesID); err != nil {
		return fmt.Errorf("verifying series platform: %w", err)
	}

	return s.store.bulkReorderSeriesPosts(ctx, seriesID, posts)
}

// ---------------------------------------------------------------------------
// Content Reviews
// ---------------------------------------------------------------------------

// CreateReviewInput holds parameters for creating a content review.
type CreateReviewInput struct {
	ContentID pgtype.UUID
	Status    string
	Feedback  string
}

// CreateReview creates a new content review.
// BUG-003: reviewer_user_id is now nullable — pass pgtype.UUID{} (zero value,
// Valid=false → NULL) when no reviewer is assigned yet (e.g. SubmitForReview).
func (s *Service) CreateReview(ctx context.Context, reviewerUserID pgtype.UUID, input CreateReviewInput) (gen.ContentReview, error) {
	if input.ContentID == (pgtype.UUID{}) {
		return gen.ContentReview{}, errors.New("content_id is required")
	}

	status := input.Status
	if status == "" {
		status = ReviewStatusPending
	}

	var feedback *string
	if input.Feedback != "" {
		feedback = &input.Feedback
	}

	return s.store.createContentReview(ctx, gen.CreateContentReviewParams{
		ContentID:      input.ContentID,
		ReviewerUserID: reviewerUserID, // zero value → NULL when no reviewer
		Status:         status,
		Feedback:       feedback,
	})
}

// GetReview retrieves a content review by ID.
func (s *Service) GetReview(ctx context.Context, id pgtype.UUID) (gen.ContentReview, error) {
	return s.store.getContentReviewByID(ctx, id)
}

// ListReviewsByContent returns all reviews for a content item.
func (s *Service) ListReviewsByContent(ctx context.Context, contentID pgtype.UUID) ([]gen.ContentReview, error) {
	return s.store.listContentReviewsByContent(ctx, contentID)
}

// ListPendingReviews returns paginated pending reviews for a platform.
func (s *Service) ListPendingReviews(ctx context.Context, limit, offset int32) ([]gen.ContentReview, error) {
	return s.store.listPendingContentReviews(ctx, limit, offset)
}

// UpdateReviewStatus updates a review's status and feedback.
// BUG-003: also records the actual reviewer's user ID on approve/reject.
func (s *Service) UpdateReviewStatus(ctx context.Context, id, reviewerUserID pgtype.UUID, status, feedback string) (gen.ContentReview, error) {
	if status == "" {
		return gen.ContentReview{}, errors.New("status is required")
	}

	var fb *string
	if feedback != "" {
		fb = &feedback
	}

	return s.store.updateContentReviewStatus(ctx, gen.UpdateContentReviewStatusParams{
		ID: id,

		ReviewerUserID: reviewerUserID, // COALESCE in SQL keeps existing if zero
		Status:         status,
		Feedback:       fb,
	})
}

// DeleteReview removes a content review by ID.
func (s *Service) DeleteReview(ctx context.Context, id pgtype.UUID) error {
	return s.store.deleteContentReview(ctx, id)
}

// ---------------------------------------------------------------------------
// Featured Posts
// ---------------------------------------------------------------------------

// FeaturePostInput holds parameters for featuring a post.
type FeaturePostInput struct {
	ContentID     pgtype.UUID
	FeaturedOrder int32
	FeaturedUntil pgtype.Timestamptz
}

// FeaturePost marks a post as featured.
func (s *Service) FeaturePost(ctx context.Context, input FeaturePostInput) (gen.ContentFeatured, error) {
	if input.ContentID == (pgtype.UUID{}) {
		return gen.ContentFeatured{}, errors.New("content_id is required")
	}

	return s.store.featureContent(ctx, gen.FeatureContentParams{
		ContentID:     input.ContentID,
		FeaturedOrder: input.FeaturedOrder,
		FeaturedUntil: input.FeaturedUntil,
	})
}

// UnfeaturePost removes a post from featured.
func (s *Service) UnfeaturePost(ctx context.Context, contentID pgtype.UUID) error {
	return s.store.unfeatureContent(ctx, contentID)
}

// ListFeaturedPosts returns paginated featured posts.
func (s *Service) ListFeaturedPosts(ctx context.Context, limit, offset int32) ([]gen.ContentFeatured, error) {
	return s.store.listFeaturedPosts(ctx, limit, offset)
}

// UpdateFeaturedPostOrder updates the display order of a featured post.
func (s *Service) UpdateFeaturedPostOrder(ctx context.Context, id pgtype.UUID, order int32) error {
	return s.store.updateFeaturedOrder(ctx, id, order)
}

// BulkUpdateFeaturedOrder transactionally updates all featured post orders
// (M3 — all-or-nothing on failure).
func (s *Service) BulkUpdateFeaturedOrder(ctx context.Context, items []struct {
	ID    pgtype.UUID
	Order int32
},
) error {
	return s.store.bulkUpdateFeaturedOrder(ctx, items)
}

// ListFeaturedBlogPostCards returns enriched featured post cards with JOINs
// to contents, users, and media (H6).
func (s *Service) ListFeaturedBlogPostCards(ctx context.Context, limit, offset int32) ([]gen.ListFeaturedBlogPostCardsRow, error) {
	return s.store.listFeaturedBlogPostCards(ctx, limit, offset)
}

// CountFeaturedBlogPostCards returns the total number of active featured blog
// posts for pagination meta.
func (s *Service) CountFeaturedBlogPostCards(ctx context.Context) (int64, error) {
	return s.store.countFeaturedBlogPostCards(ctx)
}

// ExpireFeaturedPosts marks all past-due featured posts as expired.
func (s *Service) ExpireFeaturedPosts(ctx context.Context) error {
	return s.store.expireFeaturedPosts(ctx)
}

// ---------------------------------------------------------------------------
// Blog Post Detail / List (enriched DTOs)
// ---------------------------------------------------------------------------

// GetBlogPostDetail returns a single blog post with author, avatar, featured
// image, space name, tags, and series navigation (C4).
// Authors can view their own drafts; super-admins can view any post.
func (s *Service) GetBlogPostDetail(ctx context.Context, slug string, viewerUserID pgtype.UUID, isSuperAdmin bool) (gen.GetBlogPostDetailRow, error) {
	return s.store.getBlogPostDetail(ctx, slug, viewerUserID, isSuperAdmin)
}

// ListPublishedBlogPosts returns published blog post cards with author and
// featured image, status-filtered in SQL (C5/H3).
// BUG-005: offset-based pagination per Rule 5 (page_size + page_offset).
// BUG-011: accepts §24-mandated filters via BlogListFilter.
func (s *Service) ListPublishedBlogPosts(ctx context.Context, f BlogListFilter, limit, offset int32) ([]gen.ListPublishedBlogPostsRow, error) {
	return s.store.listPublishedBlogPosts(ctx, f, limit, offset)
}

// CountPublishedBlogPosts returns the total number of published blog posts
// for pagination meta.
// BUG-011: accepts the same §24 filters as ListPublishedBlogPosts.
func (s *Service) CountPublishedBlogPosts(ctx context.Context, f BlogListFilter) (int64, error) {
	return s.store.countPublishedBlogPosts(ctx, f)
}

// ListPendingReviewSummaries returns joined review-queue rows with post
// title/slug/author/excerpt for the admin review-queue (C3).
func (s *Service) ListPendingReviewSummaries(ctx context.Context, limit, offset int32) ([]gen.ListPendingReviewSummariesRow, error) {
	return s.store.listPendingReviewSummaries(ctx, limit, offset)
}

// ListAdminBlogPosts returns all-status blog post cards with author name
// for the admin /blog table (BUG-B). Offset-paginated per Rule 5.
func (s *Service) ListAdminBlogPosts(ctx context.Context, limit, offset int32) ([]gen.ListAdminBlogPostsRow, error) {
	return s.store.listAdminBlogPosts(ctx, limit, offset)
}

// ---------------------------------------------------------------------------
// RSS Feed
// ---------------------------------------------------------------------------

// RSSPost holds the data needed for an RSS feed item.
type RSSPost struct {
	ID              pgtype.UUID        `json:"id"`
	Title           string             `json:"title"`
	Slug            string             `json:"slug"`
	BodyHTML        string             `json:"body_html"`
	AuthorID        pgtype.UUID        `json:"author_id"`
	PublishedAt     pgtype.Timestamptz `json:"published_at"`
	CreatedAt       pgtype.Timestamptz `json:"created_at"`
	ReadTimeMinutes int32              `json:"read_time_minutes"`
}

// ListPublishedPostsForRSS returns published blog posts for RSS feed generation.
func (s *Service) ListPublishedPostsForRSS(ctx context.Context, limit int32) ([]gen.ListPublishedPostsForRSSRow, error) {
	return s.store.listPublishedPostsForRSS(ctx, limit)
}
