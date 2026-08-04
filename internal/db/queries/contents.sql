-- Content, revision, slug history, pinned posts, JSON-LD templates, and scores.

-- =====================
-- Contents
-- =====================

-- name: CreateContent :one
INSERT INTO contents (
    tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
    field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
    canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
    status, visibility, read_time_minutes, publish_at, published_at, guide_category
) VALUES (
    @tenant_id, @author_id, sqlc.narg(organization_id), sqlc.narg(space_id), @content_type, @title, @slug, @body, @body_html,
    @field_values, sqlc.narg(content_hash), sqlc.narg(geo_space_id),
    sqlc.narg(header_image_id), sqlc.narg(featured_image_id),
    sqlc.narg(canonical_url), sqlc.narg(meta_title), sqlc.narg(meta_description),
    sqlc.narg(og_image_url), @seo_indexable,
    @status, @visibility, @read_time_minutes, sqlc.narg(publish_at), sqlc.narg(published_at),
    sqlc.narg(guide_category)
)
RETURNING id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
          status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at, guide_category;

-- name: GetContentByID :one
SELECT id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
       status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at, guide_category
FROM contents
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: GetContentByHash :one
-- Duplicate-detection check (Rule 12 / BL-006). Returns a conflicting row
-- with the same (tenant_id, content_hash), excluding the row being updated.
SELECT id, tenant_id, author_id, content_hash
FROM contents
WHERE tenant_id = @tenant_id
  AND content_hash = @content_hash
  AND deleted_at IS NULL
  AND id <> @exclude_id;

-- name: GetContentBySlug :one
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count,
       (SELECT COUNT(*) FROM comments WHERE content_id = c.id AND deleted_at IS NULL) AS comment_count,
       c.share_count,
       (SELECT COUNT(*) FROM reactions WHERE entity_id = c.id AND entity_type = 'content') AS reshare_count,
       c.publish_at, c.published_at,
       c.created_at, c.updated_at, c.guide_category
FROM contents c
WHERE c.slug = @slug AND c.tenant_id = @tenant_id AND c.deleted_at IS NULL;

-- name: ListContentsByTenant :many
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
       status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at, guide_category
FROM contents
WHERE tenant_id = @tenant_id
  AND deleted_at IS NULL
  AND auto_hidden = false
  AND (@q::text IS NULL OR title ILIKE '%' || @q || '%' OR slug ILIKE '%' || @q || '%' OR body ILIKE '%' || @q || '%')
  AND (created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: ListContentsByType :many
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
       status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at, guide_category
FROM contents
WHERE tenant_id = @tenant_id AND content_type = @content_type
  AND deleted_at IS NULL
  AND auto_hidden = false
  AND (created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: ListContentsBySpace :many
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
       status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at, guide_category
FROM contents
WHERE tenant_id = @tenant_id AND space_id = @space_id
  AND deleted_at IS NULL
  AND auto_hidden = false
  AND (created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: SearchContentsBySpace :many
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
       status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at, guide_category
FROM contents
WHERE tenant_id = @tenant_id AND space_id = @space_id
  AND deleted_at IS NULL
  AND auto_hidden = false
  AND title ILIKE '%' || @query || '%'
ORDER BY created_at DESC, id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateContent :one
UPDATE contents
SET title = @title, slug = @slug, body = @body, body_html = @body_html,
    field_values = @field_values, content_hash = sqlc.narg(content_hash),
    geo_space_id = sqlc.narg(geo_space_id), canonical_url = sqlc.narg(canonical_url),
    header_image_id = sqlc.narg(header_image_id),
    featured_image_id = sqlc.narg(featured_image_id),
    meta_title = sqlc.narg(meta_title),
    meta_description = sqlc.narg(meta_description),
    og_image_url = sqlc.narg(og_image_url),
    seo_indexable = COALESCE(sqlc.narg(seo_indexable), seo_indexable),
    status = @status, visibility = @visibility, read_time_minutes = @read_time_minutes,
    publish_at = sqlc.narg(publish_at), published_at = sqlc.narg(published_at),
    is_locked = @is_locked, is_pinned = @is_pinned,
    organization_id = sqlc.narg(organization_id), guide_category = sqlc.narg(guide_category),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, author_id, organization_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
          status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at, guide_category;

-- name: UpdateContentStatus :one
UPDATE contents
SET status = @status, updated_at = now(),
    published_at = CASE WHEN @status = 'published' AND published_at IS NULL THEN now() ELSE published_at END
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
          status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at, guide_category;

-- name: ScheduleContent :one
-- Sets status='scheduled' and publish_at without touching title/body/other
-- fields (catalog: POST /content/:id/schedule). The scheduled_publish Asynq
-- task (enqueued by the caller with ProcessAt=publish_at) flips status to
-- 'published' when the time arrives.
UPDATE contents
SET status = 'scheduled', publish_at = @publish_at, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL
RETURNING id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
          status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at, guide_category;

-- name: IncrementContentView :exec
UPDATE contents SET view_count = view_count + 1 WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: RecordContentView :exec
-- Inserts a deduplicated view record into user_content_views (Section 14).
-- The PK deduplicates per user/session per day; ON CONFLICT does nothing.
-- country_code is resolved server-side from the client IP via GeoIP.
INSERT INTO user_content_views (content_id, user_id, session_id, country_code, viewed_at)
VALUES (@content_id, @user_id, @session_id, @country_code, now())
ON CONFLICT DO NOTHING;

-- name: IncrementContentCommentCount :exec
-- Increments the denormalized comment_count on a content row. Called
-- synchronously when a comment is created so feed cards (which read
-- c.comment_count) show accurate counts without waiting for the async
-- scoring pipeline. Ref: Section 32 (feed reads contents.comment_count).
UPDATE contents SET comment_count = comment_count + 1
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: DecrementContentCommentCount :exec
-- Decrements comment_count when a comment is soft-deleted/removed.
UPDATE contents SET comment_count = GREATEST(comment_count - 1, 0)
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: DeleteContent :exec
UPDATE contents SET deleted_at = now(), updated_at = now() WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- =====================
-- Content Revisions
-- =====================

-- name: CreateContentRevision :one
INSERT INTO content_revisions (content_id, field_values, title, body, body_html, edited_by)
VALUES (@content_id, @field_values, @title, @body, @body_html, @edited_by)
RETURNING id, content_id, field_values, title, body, body_html, edited_by, created_at;

-- name: ListContentRevisions :many
SELECT id, content_id, field_values, title, body, body_html, edited_by, created_at
FROM content_revisions
WHERE content_id = @content_id
ORDER BY created_at DESC, id DESC;

-- name: GetContentRevision :one
SELECT id, content_id, field_values, title, body, body_html, edited_by, created_at
FROM content_revisions
WHERE id = @id AND content_id = @content_id;

-- =====================
-- Slug History
-- =====================

-- name: CreateSlugHistory :one
INSERT INTO slug_history (entity_type, entity_id, old_slug, new_slug)
VALUES (@entity_type, @entity_id, @old_slug, @new_slug)
RETURNING id, entity_type, entity_id, old_slug, new_slug, created_at;

-- name: GetSlugRedirect :one
SELECT id, entity_type, entity_id, old_slug, new_slug, created_at
FROM slug_history
WHERE entity_type = @entity_type AND old_slug = @old_slug
ORDER BY created_at DESC
LIMIT 1;

-- =====================
-- Space Pinned Posts
-- =====================

-- name: PinContentToSpace :one
INSERT INTO space_pinned_posts (space_id, content_id, pinned_by, position)
VALUES (@space_id, @content_id, @pinned_by, @position)
RETURNING id, space_id, content_id, pinned_by, position, created_at;

-- name: UnpinContentFromSpace :exec
DELETE FROM space_pinned_posts WHERE space_id = @space_id AND content_id = @content_id;

-- name: ListPinnedContent :many
SELECT spp.id, spp.space_id, spp.content_id, spp.pinned_by, spp.position, spp.created_at
FROM space_pinned_posts spp
WHERE spp.space_id = @space_id
ORDER BY spp.position ASC, spp.created_at ASC;

-- name: IsContentPinned :one
SELECT EXISTS(SELECT 1 FROM space_pinned_posts WHERE space_id = @space_id AND content_id = @content_id) AS exists;

-- =====================
-- JSON-LD Templates
-- =====================

-- name: GetJSONLDTemplate :one
SELECT id, tenant_id, content_type, template, is_active, created_at, updated_at
FROM content_type_jsonld_templates
WHERE tenant_id = @tenant_id AND content_type = @content_type AND is_active = true;

-- name: ListJSONLDTemplates :many
SELECT id, tenant_id, content_type, template, is_active, created_at, updated_at
FROM content_type_jsonld_templates
WHERE tenant_id = @tenant_id
  AND (@q::text = '' OR content_type ILIKE '%' || @q || '%' OR template::text ILIKE '%' || @q || '%')
ORDER BY created_at ASC, id ASC;

-- name: UpsertJSONLDTemplate :one
INSERT INTO content_type_jsonld_templates (tenant_id, content_type, template, is_active)
VALUES (@tenant_id, @content_type, @template, @is_active)
ON CONFLICT (tenant_id, content_type) DO UPDATE
SET template = EXCLUDED.template, is_active = EXCLUDED.is_active, updated_at = now()
RETURNING id, tenant_id, content_type, template, is_active, created_at, updated_at;

-- name: DeleteJSONLDTemplate :exec
DELETE FROM content_type_jsonld_templates WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Content Scores
-- =====================

-- name: UpsertContentScore :one
INSERT INTO content_scores (content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score)
VALUES (@content_id, @vote_score, @comment_count, @view_count, @bookmark_count, @velocity_score)
ON CONFLICT (content_id) DO UPDATE
SET vote_score = EXCLUDED.vote_score, comment_count = EXCLUDED.comment_count,
    view_count = EXCLUDED.view_count, bookmark_count = EXCLUDED.bookmark_count,
    velocity_score = EXCLUDED.velocity_score, last_scored_at = now()
RETURNING content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score, relevance_embedding, last_scored_at;

-- name: EnsureContentScore :exec
-- Creates a zero-default content_scores row if one does not yet exist.
-- Called on content publish and by the engagement worker so that INNER JOIN
-- feed queries (explore/top/rising/space/profile) never exclude content for
-- lacking a score row, and so the worker can increment counts in place.
-- ON CONFLICT DO NOTHING preserves any existing scores.
INSERT INTO content_scores (content_id)
VALUES (@content_id)
ON CONFLICT (content_id) DO NOTHING;

-- name: IncrementScoreVote :exec
-- Atomically increments vote_score by delta. Used by the engagement worker
-- to avoid the read-modify-write race that loses deltas when concurrent
-- events (vote, comment, bookmark, view) overwrite each other's columns.
UPDATE content_scores
SET vote_score = vote_score + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreComment :exec
-- Atomically increments comment_count by delta.
UPDATE content_scores
SET comment_count = comment_count + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreView :exec
-- Atomically increments view_count by delta.
UPDATE content_scores
SET view_count = view_count + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: IncrementScoreBookmark :exec
-- Atomically increments bookmark_count by delta.
UPDATE content_scores
SET bookmark_count = bookmark_count + @delta, last_scored_at = now()
WHERE content_id = @content_id;

-- name: GetContentScore :one
SELECT content_id, vote_score, comment_count, view_count, bookmark_count, velocity_score, relevance_embedding, last_scored_at
FROM content_scores
WHERE content_id = @content_id;

-- =====================
-- Content Tags
-- =====================

-- name: AddContentTag :exec
INSERT INTO content_tags (content_id, tag_id)
VALUES (@content_id, @tag_id)
ON CONFLICT (content_id, tag_id) DO NOTHING;

-- name: RemoveContentTag :exec
DELETE FROM content_tags WHERE content_id = @content_id AND tag_id = @tag_id;

-- name: ListContentTags :many
SELECT t.id, t.name, t.slug, t.description, t.color, t.follower_count
FROM content_tags ct
JOIN tags t ON t.id = ct.tag_id
WHERE ct.content_id = @content_id
ORDER BY t.name;

-- name: GetContentSpaces :many
-- Returns all space assignments for a content item (Section 14: content_spaces).
-- Used by feed fan-out (Section 32) to distribute a post to every assigned space.
SELECT cs.space_id, cs.is_primary
FROM content_spaces cs
WHERE cs.content_id = @content_id
ORDER BY cs.is_primary DESC, cs.created_at ASC;

-- name: DeleteContentSpaces :exec
-- Removes all space assignments for a content item (Section 14).
-- Used by UpdateContentSpaces to replace the full assignment set.
DELETE FROM content_spaces WHERE content_id = @content_id;

-- name: AddContentSpace :exec
-- Inserts a single space assignment for a content item (Section 14).
-- is_primary must be true for exactly one row per content item.
INSERT INTO content_spaces (content_id, space_id, is_primary)
VALUES (@content_id, @space_id, @is_primary);

-- name: SetContentSpaceID :exec
-- Updates only the space_id column on a content item. Used by the auto-assign
-- feature to set the primary space after keyword matching.
UPDATE contents SET space_id = @space_id, updated_at = now()
WHERE id = @content_id;

-- name: ListContentByTag :many
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at
FROM contents c
JOIN content_tags ct ON ct.content_id = c.id
WHERE ct.tag_id = @tag_id AND c.tenant_id = @tenant_id AND c.status = 'published'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
  AND (c.created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY c.created_at DESC, c.id DESC
LIMIT @page_limit;

-- =====================
-- Content Shares
-- =====================

-- name: CreateContentShare :one
INSERT INTO content_shares (tenant_id, content_id, user_id, platform)
VALUES (@tenant_id, @content_id, @user_id, @platform)
RETURNING id, tenant_id, content_id, user_id, platform, created_at;

-- name: CountContentShares :one
SELECT COUNT(*)::int AS count
FROM content_shares
WHERE tenant_id = @tenant_id AND content_id = @content_id;

-- =====================
-- Bookmarks (polymorphic)
-- =====================

-- name: CreateBookmark :one
INSERT INTO bookmarks (tenant_id, user_id, entity_type, entity_id, folder, folder_id)
VALUES (@tenant_id, @user_id, @entity_type, @entity_id, @folder, @folder_id)
ON CONFLICT (user_id, entity_type, entity_id) DO UPDATE SET folder = EXCLUDED.folder, folder_id = EXCLUDED.folder_id
RETURNING id, tenant_id, user_id, entity_type, entity_id, folder, folder_id, created_at;

-- name: SetBookmarkFolder :exec
UPDATE bookmarks SET folder_id = @folder_id
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: DeleteBookmark :exec
DELETE FROM bookmarks
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: IsBookmarked :one
SELECT EXISTS(
    SELECT 1 FROM bookmarks
    WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id
) AS is_bookmarked;

-- name: ListBookmarksByUser :many
SELECT b.id, b.tenant_id, b.user_id, b.entity_type, b.entity_id, b.folder, b.folder_id, b.created_at
FROM bookmarks b
WHERE b.user_id = @user_id AND b.tenant_id = @tenant_id
ORDER BY b.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListBookmarksByFolder :many
SELECT b.id, b.tenant_id, b.user_id, b.entity_type, b.entity_id, b.folder, b.folder_id, b.created_at
FROM bookmarks b
WHERE b.user_id = @user_id AND b.tenant_id = @tenant_id AND b.folder_id = @folder_id
ORDER BY b.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountBookmarksByUser :one
SELECT COUNT(*)::int AS count
FROM bookmarks
WHERE user_id = @user_id AND tenant_id = @tenant_id;

-- name: ListContentsByAuthorProfile :many
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, meta_title, meta_description, og_image_url, seo_indexable,
       status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at, guide_category
FROM contents
WHERE author_profile_id = @author_profile_id AND tenant_id = @tenant_id AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPublishedContentByAuthor :many
-- Public profile Posts tab (GET /users/{id}/content): a user's PUBLISHED PUBLIC
-- content, newest first, paginated. author_id is the user id (FK users.id).
-- LEFT JOINs organizations so dual-surface posts carry the "on behalf of" badge.
-- Pinned posts (is_pinned = true) are sorted to the top so the profile
-- surfaces the author's featured/top content first.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
       c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at,
       o.name AS org_name, o.slug AS org_slug, o.logo_media_id AS org_logo_media_id
FROM contents c
LEFT JOIN organizations o ON o.id = c.organization_id AND o.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.author_id = @author_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
ORDER BY c.is_pinned DESC, c.published_at DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPublishedContentByOrganization :many
-- Org profile Posts tab (GET /organizations/{id}/content): the org's PUBLISHED
-- PUBLIC content, newest first, paginated. Keyed by organization_id (ADR-062).
-- author_id (the human) is projected for accountability; org badge fields via join.
SELECT c.id, c.tenant_id, c.author_id, c.organization_id, c.space_id, c.content_type,
       c.title, c.slug, c.body, c.body_html, c.field_values, c.content_hash,
       c.geo_space_id, c.header_image_id, c.featured_image_id, c.canonical_url,
       c.meta_title, c.meta_description, c.og_image_url, c.seo_indexable,
       c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count,
       c.publish_at, c.published_at, c.created_at, c.updated_at,
       o.name AS org_name, o.slug AS org_slug, o.logo_media_id AS org_logo_media_id
FROM contents c
LEFT JOIN organizations o ON o.id = c.organization_id AND o.tenant_id = c.tenant_id
WHERE c.tenant_id = @tenant_id
  AND c.organization_id = @organization_id
  AND c.status = 'published'
  AND c.visibility = 'public'
  AND c.deleted_at IS NULL
  AND c.auto_hidden = false
ORDER BY c.published_at DESC NULLS LAST, c.created_at DESC, c.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: PublishScheduledContent :execrows
UPDATE contents
SET status = 'published',
    published_at = COALESCE(published_at, now()),
    updated_at = now()
WHERE id = @id
  AND tenant_id = @tenant_id
  AND status = 'scheduled'
  AND publish_at <= now();

-- name: ListScheduledContentReady :many
SELECT id, tenant_id
FROM contents
WHERE status = 'scheduled'
  AND publish_at IS NOT NULL
  AND publish_at <= now()
ORDER BY publish_at ASC
LIMIT 100;

-- name: GetContentTitleAndBody :one
SELECT COALESCE(title, '') AS title, COALESCE(body, '') AS body
FROM contents
WHERE id = @id AND tenant_id = @tenant_id AND deleted_at IS NULL;

-- name: ListPublishedSlugsForSitemap :many
SELECT slug, updated_at
FROM contents
WHERE tenant_id = @tenant_id AND status = 'published'
ORDER BY updated_at DESC
LIMIT 10000;

-- name: ListPublishedContentForRSS :many
SELECT title, slug, created_at
FROM contents
WHERE tenant_id = @tenant_id AND status = 'published'
ORDER BY created_at DESC
LIMIT 50;

-- name: ListPublishedContentForLLMsTxt :many
SELECT title, slug
FROM contents
WHERE tenant_id = @tenant_id AND status = 'published'
ORDER BY created_at DESC
LIMIT 1000;

-- name: ListPublishedContentForDigest :many
SELECT title, slug, created_at
FROM contents
WHERE tenant_id = @tenant_id AND status = 'published'
ORDER BY created_at DESC
LIMIT 10;

-- name: ArchiveContent :execrows
UPDATE contents SET status = 'archived', updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND status != 'archived';

-- name: GetPublicAuthorAnalytics :one
-- Public profile Analytics tab (GET /users/{id}/analytics): aggregates
-- published+public content stats for the profile owner. Soft-deleted and
-- non-public/draft content is excluded. total_likes counts reactions on the
-- author's published+public content. Follower/following counts come from the
-- users table / follows subquery so the analytics response is self-contained.
-- Uses subqueries for content aggregates so the user row is always returned
-- even when the user has zero published posts (no GROUP BY needed).
SELECT
    COALESCE((
        SELECT SUM(c.view_count)::int FROM contents c
        WHERE c.tenant_id = @tenant_id
          AND c.author_id = @author_id
          AND c.status = 'published'
          AND c.visibility = 'public'
          AND c.deleted_at IS NULL
    ), 0)::int AS total_views,
    COALESCE((
        SELECT SUM(c.share_count)::int FROM contents c
        WHERE c.tenant_id = @tenant_id
          AND c.author_id = @author_id
          AND c.status = 'published'
          AND c.visibility = 'public'
          AND c.deleted_at IS NULL
    ), 0)::int AS total_shares,
    COALESCE((
        SELECT COUNT(c.id)::int FROM contents c
        WHERE c.tenant_id = @tenant_id
          AND c.author_id = @author_id
          AND c.status = 'published'
          AND c.visibility = 'public'
          AND c.deleted_at IS NULL
    ), 0)::int AS total_posts,
    COALESCE((
        SELECT COUNT(*)::int FROM reactions r
        WHERE r.tenant_id = @tenant_id
          AND r.entity_type = 'content'
          AND r.entity_id IN (
              SELECT c2.id FROM contents c2
              WHERE c2.tenant_id = @tenant_id
                AND c2.author_id = @author_id
                AND c2.status = 'published'
                AND c2.visibility = 'public'
                AND c2.deleted_at IS NULL
          )
    ), 0)::int AS total_likes,
    u.follower_count::int AS total_followers,
    (SELECT COUNT(*)::int FROM follows f
      WHERE f.tenant_id = @tenant_id AND f.follower_id = u.id) AS total_following
FROM users u
WHERE u.id = @author_id AND u.tenant_id = @tenant_id;
