-- Blog Extension — series, content reviews, featured posts, RSS feed.

-- =====================
-- Blog Series
-- =====================

-- name: CreateBlogSeries :one
INSERT INTO blog_series (
    tenant_id, title, slug, description, cover_image_id, author_user_id
) VALUES (
    @tenant_id, @title, @slug, sqlc.narg(description),
    sqlc.narg(cover_image_id), @author_user_id
)
RETURNING id, tenant_id, title, slug, description, cover_image_id,
          author_user_id, status, created_at, updated_at;

-- name: GetBlogSeriesByID :one
SELECT id, tenant_id, title, slug, description, cover_image_id,
       author_user_id, status, created_at, updated_at
FROM blog_series
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetBlogSeriesBySlug :one
SELECT id, tenant_id, title, slug, description, cover_image_id,
       author_user_id, status, created_at, updated_at
FROM blog_series
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: ListBlogSeriesByTenant :many
SELECT id, tenant_id, title, slug, description, cover_image_id,
       author_user_id, status, created_at, updated_at
FROM blog_series
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListBlogSeriesByAuthor :many
SELECT id, tenant_id, title, slug, description, cover_image_id,
       author_user_id, status, created_at, updated_at
FROM blog_series
WHERE tenant_id = @tenant_id AND author_user_id = @author_user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateBlogSeries :one
UPDATE blog_series
SET title = @title, description = sqlc.narg(description),
    cover_image_id = sqlc.narg(cover_image_id), status = @status,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, title, slug, description, cover_image_id,
          author_user_id, status, created_at, updated_at;

-- name: DeleteBlogSeries :exec
DELETE FROM blog_series
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Blog Series Posts
-- =====================

-- name: AddPostToSeries :exec
INSERT INTO blog_series_posts (series_id, content_id, position)
VALUES (@series_id, @content_id, @position)
ON CONFLICT (series_id, content_id) DO UPDATE
    SET position = @position;

-- name: RemovePostFromSeries :exec
DELETE FROM blog_series_posts
WHERE series_id = @series_id AND content_id = @content_id;

-- name: ListPostsInSeries :many
SELECT bsp.series_id, bsp.content_id, bsp.position,
       c.id AS content_id, c.title, c.slug, c.status, c.created_at
FROM blog_series_posts bsp
JOIN contents c ON c.id = bsp.content_id
WHERE bsp.series_id = @series_id
ORDER BY bsp.position ASC;

-- name: ReorderSeriesPosts :exec
UPDATE blog_series_posts
SET position = @position
WHERE series_id = @series_id AND content_id = @content_id;

-- name: DeleteSeriesPosts :exec
DELETE FROM blog_series_posts
WHERE series_id = @series_id;

-- =====================
-- Content Reviews
-- =====================

-- name: CreateContentReview :one
INSERT INTO content_reviews (
    tenant_id, content_id, reviewer_user_id, status, feedback
) VALUES (
    @tenant_id, @content_id, @reviewer_user_id, @status, sqlc.narg(feedback)
)
RETURNING id, tenant_id, content_id, reviewer_user_id, status, feedback,
          created_at, updated_at;

-- name: GetContentReviewByID :one
SELECT id, tenant_id, content_id, reviewer_user_id, status, feedback,
       created_at, updated_at
FROM content_reviews
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListContentReviewsByContent :many
SELECT id, tenant_id, content_id, reviewer_user_id, status, feedback,
       created_at, updated_at
FROM content_reviews
WHERE content_id = @content_id AND tenant_id = @tenant_id
ORDER BY created_at DESC;

-- name: ListPendingContentReviews :many
SELECT id, tenant_id, content_id, reviewer_user_id, status, feedback,
       created_at, updated_at
FROM content_reviews
WHERE tenant_id = @tenant_id AND status = 'pending'
ORDER BY created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateContentReviewStatus :one
UPDATE content_reviews
SET status = @status, feedback = sqlc.narg(feedback), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, content_id, reviewer_user_id, status, feedback,
          created_at, updated_at;

-- name: DeleteContentReview :exec
DELETE FROM content_reviews
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Content Featured
-- =====================

-- name: FeatureContent :one
INSERT INTO content_featured (
    tenant_id, content_id, featured_order, featured_until
) VALUES (
    @tenant_id, @content_id, @featured_order, sqlc.narg(featured_until)
)
ON CONFLICT (tenant_id, content_id) DO UPDATE
    SET featured_order = @featured_order,
        featured_until = sqlc.narg(featured_until)
RETURNING id, tenant_id, content_id, featured_order, featured_until, created_at;

-- name: UnfeatureContent :exec
DELETE FROM content_featured
WHERE tenant_id = @tenant_id AND content_id = @content_id;

-- name: ListFeaturedPosts :many
SELECT cf.id, cf.tenant_id, cf.content_id, cf.featured_order,
       cf.featured_until, cf.created_at
FROM content_featured cf
WHERE cf.tenant_id = @tenant_id
  AND (cf.featured_until IS NULL OR cf.featured_until > now())
ORDER BY cf.featured_order ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateFeaturedOrder :exec
UPDATE content_featured
SET featured_order = @featured_order
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ExpireFeaturedPosts :exec
UPDATE content_featured
SET featured_until = now()
WHERE featured_until IS NOT NULL AND featured_until <= now()
  AND featured_until > '2000-01-01';

-- =====================
-- RSS Feed Queries
-- =====================

-- name: ListPublishedPostsForRSS :many
SELECT c.id, c.title, c.slug, c.body_html, c.author_id,
       c.published_at, c.created_at, c.read_time_minutes
FROM contents c
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.content_type = 'blog_post'
ORDER BY c.published_at DESC
LIMIT @page_limit;
