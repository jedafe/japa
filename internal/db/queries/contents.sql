-- Content, revision, slug history, pinned posts, JSON-LD templates, and scores.

-- =====================
-- Contents
-- =====================

-- name: CreateContent :one
INSERT INTO contents (
    tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
    field_values, content_hash, geo_space_id, canonical_url, status, visibility,
    read_time_minutes, publish_at, published_at
) VALUES (
    @tenant_id, @author_id, sqlc.narg(space_id), @content_type, @title, @slug, @body, @body_html,
    @field_values, sqlc.narg(content_hash), sqlc.narg(geo_space_id), sqlc.narg(canonical_url),
    @status, @visibility, @read_time_minutes, sqlc.narg(publish_at), sqlc.narg(published_at)
)
RETURNING id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at;

-- name: GetContentByID :one
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at
FROM contents
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetContentBySlug :one
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at
FROM contents
WHERE slug = @slug AND tenant_id = @tenant_id;

-- name: ListContentsByTenant :many
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at
FROM contents
WHERE tenant_id = @tenant_id
  AND (created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: ListContentsByType :many
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at
FROM contents
WHERE tenant_id = @tenant_id AND content_type = @content_type
  AND (created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: ListContentsBySpace :many
SELECT id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
       field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
       canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
       view_count, comment_count, share_count, reshare_count, publish_at, published_at,
       created_at, updated_at
FROM contents
WHERE tenant_id = @tenant_id AND space_id = @space_id
  AND (created_at < @cursor_created_at OR (@cursor_created_at IS NULL))
ORDER BY created_at DESC, id DESC
LIMIT @page_limit;

-- name: UpdateContent :one
UPDATE contents
SET title = @title, slug = @slug, body = @body, body_html = @body_html,
    field_values = @field_values, content_hash = sqlc.narg(content_hash),
    geo_space_id = sqlc.narg(geo_space_id), canonical_url = sqlc.narg(canonical_url),
    status = @status, visibility = @visibility, read_time_minutes = @read_time_minutes,
    publish_at = sqlc.narg(publish_at), published_at = sqlc.narg(published_at),
    is_locked = @is_locked, is_pinned = @is_pinned, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at;

-- name: UpdateContentStatus :one
UPDATE contents
SET status = @status, updated_at = now(),
    published_at = CASE WHEN @status = 'published' AND published_at IS NULL THEN now() ELSE published_at END
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, author_id, space_id, content_type, title, slug, body, body_html,
          field_values, content_hash, geo_space_id, header_image_id, featured_image_id,
          canonical_url, status, visibility, is_locked, is_pinned, read_time_minutes,
          view_count, comment_count, share_count, reshare_count, publish_at, published_at,
          created_at, updated_at;

-- name: IncrementContentView :exec
UPDATE contents SET view_count = view_count + 1 WHERE id = @id AND tenant_id = @tenant_id;

-- name: DeleteContent :exec
DELETE FROM contents WHERE id = @id AND tenant_id = @tenant_id;

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

-- name: ListContentByTag :many
SELECT c.id, c.tenant_id, c.author_id, c.space_id, c.content_type, c.title, c.slug, c.body, c.body_html,
       c.field_values, c.content_hash, c.geo_space_id, c.header_image_id, c.featured_image_id,
       c.canonical_url, c.status, c.visibility, c.is_locked, c.is_pinned, c.read_time_minutes,
       c.view_count, c.comment_count, c.share_count, c.reshare_count, c.publish_at, c.published_at,
       c.created_at, c.updated_at
FROM contents c
JOIN content_tags ct ON ct.content_id = c.id
WHERE ct.tag_id = @tag_id AND c.tenant_id = @tenant_id AND c.status = 'published'
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
INSERT INTO bookmarks (tenant_id, user_id, entity_type, entity_id, folder)
VALUES (@tenant_id, @user_id, @entity_type, @entity_id, @folder)
ON CONFLICT (user_id, entity_type, entity_id) DO UPDATE SET folder = EXCLUDED.folder
RETURNING id, tenant_id, user_id, entity_type, entity_id, folder, created_at;

-- name: DeleteBookmark :exec
DELETE FROM bookmarks
WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- name: IsBookmarked :one
SELECT EXISTS(
    SELECT 1 FROM bookmarks
    WHERE user_id = @user_id AND entity_type = @entity_type AND entity_id = @entity_id
) AS is_bookmarked;

-- name: ListBookmarksByUser :many
SELECT b.id, b.tenant_id, b.user_id, b.entity_type, b.entity_id, b.folder, b.created_at
FROM bookmarks b
WHERE b.user_id = @user_id AND b.tenant_id = @tenant_id
ORDER BY b.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountBookmarksByUser :one
SELECT COUNT(*)::int AS count
FROM bookmarks
WHERE user_id = @user_id AND tenant_id = @tenant_id;
