-- Blog Extension — series, content reviews, featured posts, RSS feed.
-- Ref: japaconnect-engineering/sections/24-blog-extension.md

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
          author_user_id, status, created_at, updated_at, deleted_at;

-- name: GetBlogSeriesByID :one
SELECT id, tenant_id, title, slug, description, cover_image_id,
       author_user_id, status, created_at, updated_at, deleted_at
FROM blog_series
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: GetBlogSeriesBySlug :one
SELECT id, tenant_id, title, slug, description, cover_image_id,
       author_user_id, status, created_at, updated_at, deleted_at
FROM blog_series
WHERE slug = @slug AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ListBlogSeriesByTenant :many
SELECT id, tenant_id, title, slug, description, cover_image_id,
       author_user_id, status, created_at, updated_at, deleted_at
FROM blog_series
WHERE tenant_id = @tenant_id AND deleted_at IS NULL
  AND (@q::text IS NULL OR title ILIKE '%' || @q || '%' OR slug ILIKE '%' || @q || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListBlogSeriesByAuthor :many
SELECT id, tenant_id, title, slug, description, cover_image_id,
       author_user_id, status, created_at, updated_at, deleted_at
FROM blog_series
WHERE tenant_id = @tenant_id AND author_user_id = @author_user_id
  AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateBlogSeries :one
UPDATE blog_series
SET title = @title, slug = sqlc.narg(slug), description = sqlc.narg(description),
    cover_image_id = sqlc.narg(cover_image_id), status = @status,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, title, slug, description, cover_image_id,
          author_user_id, status, created_at, updated_at, deleted_at;

-- name: DeleteBlogSeries :exec
-- Soft-delete (Rule 11): blog_series is user-authored content.
UPDATE blog_series
SET deleted_at = now(), status = 'archived', updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- =====================
-- Blog Series Posts
-- =====================

-- name: AddPostToSeries :exec
-- Auto-appends at the next available position (MAX+1, or 0 for the first
-- post). This avoids colliding with the unique constraint
-- idx_blog_series_posts_position on (series_id, position) when another
-- content already occupies the client-supplied position. Reordering is
-- handled by the dedicated ReorderSeriesPosts endpoint.
INSERT INTO blog_series_posts (series_id, content_id, position)
VALUES (
    @series_id,
    @content_id,
    COALESCE((SELECT MAX(position) + 1 FROM blog_series_posts WHERE series_id = @series_id), 0)
)
ON CONFLICT (series_id, content_id) DO NOTHING;

-- name: RemovePostFromSeries :exec
DELETE FROM blog_series_posts
WHERE series_id = @series_id AND content_id = @content_id;

-- name: InsertSeriesPostWithPosition :exec
-- Used by BulkReorderSeriesPosts after deleting all existing posts, so
-- there is no risk of colliding with the (series_id, position) unique
-- constraint — the caller controls the full position sequence.
INSERT INTO blog_series_posts (series_id, content_id, position)
VALUES (@series_id, @content_id, @position)
ON CONFLICT (series_id, content_id) DO UPDATE
    SET position = @position;

-- name: ListPostsInSeries :many
-- H5: filter deleted_at on contents; tenant-scoped via series ownership.
SELECT bsp.series_id, bsp.content_id, bsp.position,
       c.id AS content_id, c.title, c.slug, c.status, c.created_at
FROM blog_series_posts bsp
JOIN contents c ON c.id = bsp.content_id
JOIN blog_series bs ON bs.id = bsp.series_id
WHERE bsp.series_id = @series_id
  AND bs.tenant_id = @tenant_id
  AND c.deleted_at IS NULL
ORDER BY bsp.position ASC;

-- name: ReorderSeriesPosts :exec
UPDATE blog_series_posts
SET position = @position
WHERE series_id = @series_id AND content_id = @content_id;

-- name: DeleteSeriesPosts :exec
DELETE FROM blog_series_posts
WHERE series_id = @series_id;

-- name: GetSeriesForContent :one
-- Returns the series_id for a given content_id, or no rows if the content
-- is not part of any series. Used by the editor load to restore the series
-- selector when editing existing content. BUG-009: tenant-scoped to
-- prevent cross-tenant information disclosure.
SELECT bsp.series_id
FROM blog_series_posts bsp
JOIN blog_series bs ON bs.id = bsp.series_id AND bs.deleted_at IS NULL
WHERE bsp.content_id = @content_id
  AND bs.tenant_id = @tenant_id;

-- name: GetContentTenantID :one
-- H7: verify a content_id belongs to the same tenant before linking to a series.
SELECT tenant_id FROM contents WHERE id = @id AND deleted_at IS NULL;

-- =====================
-- Content Reviews
-- =====================

-- name: CreateContentReview :one
INSERT INTO content_reviews (
    tenant_id, content_id, reviewer_user_id, status, feedback
) VALUES (
    @tenant_id, @content_id, sqlc.narg(reviewer_user_id), @status, sqlc.narg(feedback)
)
RETURNING id, tenant_id, content_id, reviewer_user_id, status, feedback,
          created_at, updated_at, deleted_at;

-- name: GetContentReviewByID :one
SELECT id, tenant_id, content_id, reviewer_user_id, status, feedback,
       created_at, updated_at, deleted_at
FROM content_reviews
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ListContentReviewsByContent :many
SELECT id, tenant_id, content_id, reviewer_user_id, status, feedback,
       created_at, updated_at, deleted_at
FROM content_reviews
WHERE content_id = @content_id AND tenant_id = @tenant_id AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: ListPendingContentReviews :many
SELECT id, tenant_id, content_id, reviewer_user_id, status, feedback,
       created_at, updated_at, deleted_at
FROM content_reviews
WHERE tenant_id = @tenant_id AND status = 'pending' AND deleted_at IS NULL
ORDER BY created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPendingReviewSummaries :many
-- C3: JOIN content_reviews → contents → users for the admin review-queue.
-- Returns the post title/slug/author/excerpt the frontend ReviewItem expects.
SELECT cr.id, cr.content_id, cr.status, cr.created_at AS submitted_at,
       cr.feedback,
       c.title, c.slug, c.author_id,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(LEFT(c.body, 200), '') AS excerpt
FROM content_reviews cr
JOIN contents c ON c.id = cr.content_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE cr.tenant_id = @tenant_id
  AND cr.status = 'pending'
  AND cr.deleted_at IS NULL
  AND c.deleted_at IS NULL
  AND c.content_type = 'blog_post'
ORDER BY cr.created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateContentReviewStatus :one
-- BUG-003: also set reviewer_user_id (nullable) so the actual reviewer is
-- recorded on approve/reject, not the submitting author placeholder.
UPDATE content_reviews
SET status = @status, feedback = sqlc.narg(feedback),
    reviewer_user_id = COALESCE(sqlc.narg(reviewer_user_id), reviewer_user_id),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, content_id, reviewer_user_id, status, feedback,
          created_at, updated_at, deleted_at;

-- name: DeleteContentReview :exec
-- Soft-delete (Rule 11 / H1): content_reviews is editorial audit history.
UPDATE content_reviews
SET deleted_at = now(), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

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

-- name: ListFeaturedBlogPostCards :many
-- H6: JOIN content_featured → contents → media → users for the featured tab.
SELECT cf.id AS featured_id, cf.content_id, cf.featured_order, cf.featured_until,
       c.title, c.slug, c.body, c.body_html, c.read_time_minutes,
       c.view_count,
       (SELECT COUNT(*)
        FROM comments
        WHERE content_id = c.id
          AND tenant_id = c.tenant_id
          AND deleted_at IS NULL
          AND status = 'published') AS comment_count,
       (SELECT COUNT(*) FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reshare_count,
       c.author_id,
       COALESCE(up.display_name, u.username, '') AS author_name,
       m.storage_bucket AS featured_storage_bucket,
       m.storage_path AS featured_storage_path,
       COALESCE(LEFT(c.body, 200), '') AS excerpt
FROM content_featured cf
JOIN contents c ON c.id = cf.content_id
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN media m ON m.id = c.featured_image_id AND m.deleted_at IS NULL
WHERE cf.tenant_id = @tenant_id
  AND (cf.featured_until IS NULL OR cf.featured_until > now())
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND c.content_type = 'blog_post'
ORDER BY cf.featured_order ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateFeaturedOrder :exec
UPDATE content_featured
SET featured_order = @featured_order
WHERE id = @id AND tenant_id = @tenant_id;

-- name: BulkUpdateFeaturedOrder :exec
-- M3: transactional bulk reorder — all-or-nothing.
UPDATE content_featured
SET featured_order = @featured_order
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ExpireFeaturedPosts :exec
UPDATE content_featured
SET featured_until = now()
WHERE featured_until IS NOT NULL AND featured_until <= now()
  AND featured_until > '2000-01-01';

-- =====================
-- Blog Post Detail / List (enriched DTOs)
-- =====================

-- name: GetBlogPostDetail :one
-- C4: enriched DTO with author name, avatar, featured image, space name,
-- tags, and series navigation (prev/next within the series).
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url,
       c.seo_indexable, c.status, c.visibility, c.is_locked, c.is_pinned,
       c.read_time_minutes, c.view_count,
       (SELECT COUNT(*)
        FROM comments
        WHERE content_id = c.id
          AND tenant_id = c.tenant_id
          AND deleted_at IS NULL
          AND status = 'published') AS comment_count,
       c.share_count,
       (SELECT COUNT(*) FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(s.name, '') AS space_name,
       m.storage_bucket AS featured_storage_bucket,
       m.storage_path AS featured_storage_path,
       COALESCE(string_agg(t.slug, ',' ORDER BY t.slug), '')::text AS tags_csv,
       bs.id AS series_id,
       bs.title AS series_title,
       bsp.position AS series_part,
       (SELECT COUNT(*) FROM blog_series_posts WHERE series_id = bs.id) AS series_total,
       COALESCE(prev_c.slug, '') AS series_prev_slug,
       COALESCE(next_c.slug, '') AS series_next_slug
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN spaces s ON s.id = c.space_id
LEFT JOIN media m ON m.id = c.featured_image_id AND m.deleted_at IS NULL
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN blog_series_posts bsp ON bsp.content_id = c.id
LEFT JOIN blog_series bs ON bs.id = bsp.series_id AND bs.deleted_at IS NULL
LEFT JOIN LATERAL (
    SELECT c3.slug
    FROM blog_series_posts bsp4
    JOIN contents c3 ON c3.id = bsp4.content_id AND c3.deleted_at IS NULL
    WHERE bsp4.series_id = bs.id AND bsp4.position < bsp.position
    ORDER BY bsp4.position DESC
    LIMIT 1
) prev_c ON true
LEFT JOIN LATERAL (
    SELECT c2.slug
    FROM blog_series_posts bsp3
    JOIN contents c2 ON c2.id = bsp3.content_id AND c2.deleted_at IS NULL
    WHERE bsp3.series_id = bs.id AND bsp3.position > bsp.position
    ORDER BY bsp3.position ASC
    LIMIT 1
) next_c ON true
WHERE c.slug = @slug AND c.tenant_id = @tenant_id
  AND c.content_type = 'blog_post'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
GROUP BY c.id, up.display_name, up.avatar_url, u.username, s.name,
         m.storage_bucket, m.storage_path,
         bs.id, bs.title, bsp.position,
         prev_c.slug, next_c.slug;

-- name: ListAdminBlogPosts :many
-- BUG-B: enriched admin blog list with author_name (the generic
-- ListContentsByType returns no author join, so the admin /blog table
-- showed "—" for every author). Admin sees all statuses (not just
-- published); deleted_at filtered per Rule 11. Offset-paginated per Rule 5.
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url,
       c.seo_indexable, c.status, c.visibility, c.is_locked, c.is_pinned,
       c.read_time_minutes, c.view_count,
       (SELECT COUNT(*) FROM comments WHERE content_id = c.id AND tenant_id = c.tenant_id AND deleted_at IS NULL) AS comment_count,
       c.share_count,
       (SELECT COUNT(*) FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       COALESCE(LEFT(c.body, 200), '') AS excerpt
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
WHERE c.tenant_id = @tenant_id
  AND c.content_type = 'blog_post'
  AND c.deleted_at IS NULL
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPublishedBlogPosts :many
-- C5/H3: published-only blog post cards with author + featured image,
-- status filter in SQL (not Go memory), deleted_at filtered.
-- BUG-005: offset-based pagination per Rule 5 (?page_size=N&page_offset=N).
-- BUG-011: §24-mandated filters — ?space_id, ?tag (slug), ?series_id,
-- ?sort=latest|top, ?author_id. sort=featured is handled by the separate
-- /blog/featured endpoint. All filters are optional (NULL = no filter).
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url,
       c.seo_indexable, c.status, c.visibility, c.is_locked, c.is_pinned,
       c.read_time_minutes, c.view_count,
       (SELECT COUNT(*)
        FROM comments
        WHERE content_id = c.id
          AND tenant_id = c.tenant_id
          AND deleted_at IS NULL
          AND status = 'published') AS comment_count,
       c.share_count,
       (SELECT COUNT(*) FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at,
       COALESCE(up.display_name, u.username, '') AS author_name,
       m.storage_bucket AS featured_storage_bucket,
       m.storage_path AS featured_storage_path,
       COALESCE(LEFT(c.body, 200), '') AS excerpt
FROM contents c
LEFT JOIN users u ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN media m ON m.id = c.featured_image_id AND m.deleted_at IS NULL
WHERE c.tenant_id = @tenant_id
  AND c.content_type = 'blog_post'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
  AND (sqlc.narg(author_id)::uuid IS NULL OR c.author_id = sqlc.narg(author_id))
  AND (sqlc.narg(series_id)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM blog_series_posts bsp
      WHERE bsp.content_id = c.id AND bsp.series_id = sqlc.narg(series_id)))
  AND (sqlc.narg(tag)::text IS NULL OR EXISTS (
      SELECT 1 FROM content_tags ct2
      JOIN tags t2 ON t2.id = ct2.tag_id
      WHERE ct2.content_id = c.id AND t2.slug = sqlc.narg(tag)))
ORDER BY
    CASE WHEN @sort::text = 'top' THEN c.view_count END DESC,
    CASE WHEN @sort::text = 'top' THEN c.reshare_count END DESC,
    c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPublishedBlogPosts :one
-- Returns the total number of published blog posts for pagination meta.
-- BUG-011: accepts the same §24 filters as ListPublishedBlogPosts so the
-- pagination total reflects the active filter set.
SELECT COUNT(*) FROM contents c
WHERE c.tenant_id = @tenant_id
  AND c.content_type = 'blog_post'
  AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (sqlc.narg(space_id)::uuid IS NULL OR c.space_id = sqlc.narg(space_id))
  AND (sqlc.narg(author_id)::uuid IS NULL OR c.author_id = sqlc.narg(author_id))
  AND (sqlc.narg(series_id)::uuid IS NULL OR EXISTS (
      SELECT 1 FROM blog_series_posts bsp
      WHERE bsp.content_id = c.id AND bsp.series_id = sqlc.narg(series_id)))
  AND (sqlc.narg(tag)::text IS NULL OR EXISTS (
      SELECT 1 FROM content_tags ct2
      JOIN tags t2 ON t2.id = ct2.tag_id
      WHERE ct2.content_id = c.id AND t2.slug = sqlc.narg(tag)));

-- =====================
-- RSS Feed Queries
-- =====================

-- name: ListPublishedPostsForRSS :many
-- H4: filter deleted_at so soft-deleted posts don't leak into the feed.
-- RSS-002: filter auto_hidden = false so spam/auto-hidden posts don't leak
-- into the public blog RSS feed (matches every query in rss.sql, §51).
-- RSS-007: NULLS LAST so a NULL published_at doesn't sort ahead of real dates.
SELECT c.id, c.title, c.slug, c.body_html, c.body, c.author_id,
       c.published_at, c.created_at, c.read_time_minutes
FROM contents c
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.content_type = 'blog_post'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST
LIMIT @page_limit;
