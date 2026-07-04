package blog

import (
	"context"
	"errors"

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
func (s *Service) CreateSeries(ctx context.Context, tenantID, authorUserID pgtype.UUID, input CreateSeriesInput) (gen.BlogSeries, error) {
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
		TenantID:     tenantID,
		Title:        input.Title,
		Slug:         input.Slug,
		Description:  description,
		CoverImageID: input.CoverImageID,
		AuthorUserID: authorUserID,
	})
}

// GetSeries retrieves a blog series by ID.
func (s *Service) GetSeries(ctx context.Context, id, tenantID pgtype.UUID) (gen.BlogSeries, error) {
	return s.store.getBlogSeriesByID(ctx, id, tenantID)
}

// GetSeriesBySlug retrieves a blog series by slug.
func (s *Service) GetSeriesBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.BlogSeries, error) {
	return s.store.getBlogSeriesBySlug(ctx, slug, tenantID)
}

// ListSeriesByTenant returns paginated series for a tenant.
func (s *Service) ListSeriesByTenant(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.BlogSeries, error) {
	return s.store.listBlogSeriesByTenant(ctx, tenantID, limit, offset)
}

// ListSeriesByAuthor returns paginated series for an author.
func (s *Service) ListSeriesByAuthor(ctx context.Context, tenantID, authorUserID pgtype.UUID, limit, offset int32) ([]gen.BlogSeries, error) {
	return s.store.listBlogSeriesByAuthor(ctx, tenantID, authorUserID, limit, offset)
}

// UpdateSeries updates a blog series.
func (s *Service) UpdateSeries(ctx context.Context, id, tenantID pgtype.UUID, title, description string, coverImageID pgtype.UUID, status string) (gen.BlogSeries, error) {
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

	return s.store.updateBlogSeries(ctx, gen.UpdateBlogSeriesParams{
		ID:           id,
		TenantID:     tenantID,
		Title:        title,
		Description:  desc,
		CoverImageID: coverImageID,
		Status:       status,
	})
}

// DeleteSeries deletes a blog series.
func (s *Service) DeleteSeries(ctx context.Context, id, tenantID pgtype.UUID) error {
	return s.store.deleteBlogSeries(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Series Posts
// ---------------------------------------------------------------------------

// AddPostToSeries adds a post to a series at the given position.
func (s *Service) AddPostToSeries(ctx context.Context, seriesID, contentID pgtype.UUID, position int32) error {
	return s.store.addPostToSeries(ctx, gen.AddPostToSeriesParams{
		SeriesID:  seriesID,
		ContentID: contentID,
		Position:  position,
	})
}

// RemovePostFromSeries removes a post from a series.
func (s *Service) RemovePostFromSeries(ctx context.Context, seriesID, contentID pgtype.UUID) error {
	return s.store.removePostFromSeries(ctx, seriesID, contentID)
}

// ListPostsInSeries returns all posts in a series ordered by position.
func (s *Service) ListPostsInSeries(ctx context.Context, seriesID pgtype.UUID) ([]gen.ListPostsInSeriesRow, error) {
	return s.store.listPostsInSeries(ctx, seriesID)
}

// ReorderPostsInSeries updates the position of a post within a series.
func (s *Service) ReorderPostsInSeries(ctx context.Context, seriesID, contentID pgtype.UUID, position int32) error {
	return s.store.reorderSeriesPosts(ctx, seriesID, contentID, position)
}

// BulkReorderSeriesPosts atomically replaces all posts in a series with the
// given ordered list, eliminating the N+1 round-trip of per-post updates.
func (s *Service) BulkReorderSeriesPosts(ctx context.Context, seriesID pgtype.UUID, posts []struct {
	ContentID pgtype.UUID
	Position  int32
},
) error {
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
func (s *Service) CreateReview(ctx context.Context, tenantID, reviewerUserID pgtype.UUID, input CreateReviewInput) (gen.ContentReview, error) {
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
		TenantID:       tenantID,
		ContentID:      input.ContentID,
		ReviewerUserID: reviewerUserID,
		Status:         status,
		Feedback:       feedback,
	})
}

// GetReview retrieves a content review by ID.
func (s *Service) GetReview(ctx context.Context, id, tenantID pgtype.UUID) (gen.ContentReview, error) {
	return s.store.getContentReviewByID(ctx, id, tenantID)
}

// ListReviewsByContent returns all reviews for a content item.
func (s *Service) ListReviewsByContent(ctx context.Context, contentID, tenantID pgtype.UUID) ([]gen.ContentReview, error) {
	return s.store.listContentReviewsByContent(ctx, contentID, tenantID)
}

// ListPendingReviews returns paginated pending reviews for a tenant.
func (s *Service) ListPendingReviews(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ContentReview, error) {
	return s.store.listPendingContentReviews(ctx, tenantID, limit, offset)
}

// UpdateReviewStatus updates a review's status and feedback.
func (s *Service) UpdateReviewStatus(ctx context.Context, id, tenantID pgtype.UUID, status, feedback string) (gen.ContentReview, error) {
	if status == "" {
		return gen.ContentReview{}, errors.New("status is required")
	}

	var fb *string
	if feedback != "" {
		fb = &feedback
	}

	return s.store.updateContentReviewStatus(ctx, gen.UpdateContentReviewStatusParams{
		ID:       id,
		TenantID: tenantID,
		Status:   status,
		Feedback: fb,
	})
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
func (s *Service) FeaturePost(ctx context.Context, tenantID pgtype.UUID, input FeaturePostInput) (gen.ContentFeatured, error) {
	if input.ContentID == (pgtype.UUID{}) {
		return gen.ContentFeatured{}, errors.New("content_id is required")
	}

	return s.store.featureContent(ctx, gen.FeatureContentParams{
		TenantID:      tenantID,
		ContentID:     input.ContentID,
		FeaturedOrder: input.FeaturedOrder,
		FeaturedUntil: input.FeaturedUntil,
	})
}

// UnfeaturePost removes a post from featured.
func (s *Service) UnfeaturePost(ctx context.Context, tenantID, contentID pgtype.UUID) error {
	return s.store.unfeatureContent(ctx, tenantID, contentID)
}

// ListFeaturedPosts returns paginated featured posts.
func (s *Service) ListFeaturedPosts(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.ContentFeatured, error) {
	return s.store.listFeaturedPosts(ctx, tenantID, limit, offset)
}

// UpdateFeaturedPostOrder updates the display order of a featured post.
func (s *Service) UpdateFeaturedPostOrder(ctx context.Context, id, tenantID pgtype.UUID, order int32) error {
	return s.store.updateFeaturedOrder(ctx, id, tenantID, order)
}

// ExpireFeaturedPosts marks all past-due featured posts as expired.
func (s *Service) ExpireFeaturedPosts(ctx context.Context) error {
	return s.store.expireFeaturedPosts(ctx)
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
func (s *Service) ListPublishedPostsForRSS(ctx context.Context, tenantID pgtype.UUID, limit int32) ([]gen.ListPublishedPostsForRSSRow, error) {
	return s.store.listPublishedPostsForRSS(ctx, tenantID, limit)
}
