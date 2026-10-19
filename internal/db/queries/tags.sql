-- name: ListTags :many
SELECT t.id, t.name, t.slug, t.description, t.rules, t.cover_media_id,
 t.logo_media_id, t.color, t.follower_count, t.content_count, t.is_system,
 t.field_values, t.created_at, t.updated_at,
 lm.storage_path AS logo_storage_path, lm.storage_bucket AS logo_storage_bucket,
 cm.storage_path AS cover_storage_path, cm.storage_bucket AS cover_storage_bucket
FROM tags t
LEFT JOIN media lm ON lm.id = t.logo_media_id
LEFT JOIN media cm ON cm.id = t.cover_media_id

ORDER BY t.name;

-- name: ListTagsPaginated :many
-- Public list with sort + offset pagination (Section 08: ?sort=popular|newest&page=1).
SELECT t.id, t.name, t.slug, t.description, t.rules, t.cover_media_id,
 t.logo_media_id, t.color, t.follower_count, t.content_count, t.is_system,
 t.field_values, t.created_at, t.updated_at,
 lm.storage_path AS logo_storage_path, lm.storage_bucket AS logo_storage_bucket,
 cm.storage_path AS cover_storage_path, cm.storage_bucket AS cover_storage_bucket
FROM tags t
LEFT JOIN media lm ON lm.id = t.logo_media_id
LEFT JOIN media cm ON cm.id = t.cover_media_id
WHERE (@q::text = '' OR t.name ILIKE '%' || @q || '%' OR t.slug ILIKE '%' || @q || '%')
ORDER BY
 CASE WHEN @sort::text = 'popular' THEN t.follower_count END DESC,
 CASE WHEN @sort::text != 'popular' THEN t.created_at END DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountTagsPaginated :one
SELECT COUNT(*)::int AS count
FROM tags
WHERE (@q::text = '' OR name ILIKE '%' || @q || '%' OR slug ILIKE '%' || @q || '%');

-- name: GetTagBySlug :one
-- Case-insensitive slug match (§08 slug-uniqueness amendment): 'Career'
-- resolves the canonical 'career' tag. ORDER BY prefers the lowercase-slug
-- row if a legacy case-variant still coexists (pre-000740 data).
SELECT t.id, t.name, t.slug, t.description, t.rules, t.cover_media_id,
 t.logo_media_id, t.color, t.follower_count, t.content_count, t.is_system,
 t.field_values, t.created_at, t.updated_at,
 lm.storage_path AS logo_storage_path, lm.storage_bucket AS logo_storage_bucket,
 cm.storage_path AS cover_storage_path, cm.storage_bucket AS cover_storage_bucket
FROM tags t
LEFT JOIN media lm ON lm.id = t.logo_media_id
LEFT JOIN media cm ON cm.id = t.cover_media_id
WHERE LOWER(t.slug) = LOWER(@slug)
ORDER BY (t.slug = LOWER(t.slug)) DESC, t.created_at
LIMIT 1;

-- name: GetTagsBySlugs :many
-- Resolve multiple tag slugs to their IDs in one query (Section 14: bulk tag_slugs).
-- Callers pass lowercase-normalized slugs (content.Slugify); LOWER() on the
-- column keeps resolution correct for legacy mixed-case rows.
SELECT id, name, slug
FROM tags
WHERE LOWER(slug) = ANY(@slugs::text[]);

-- name: GetTagByID :one
-- Used by DeleteTag guard to check is_system before deletion.
SELECT t.id, t.name, t.slug, t.description, t.rules, t.cover_media_id,
 t.logo_media_id, t.color, t.follower_count, t.content_count, t.is_system,
 t.field_values, t.created_at, t.updated_at,
 lm.storage_path AS logo_storage_path, lm.storage_bucket AS logo_storage_bucket,
 cm.storage_path AS cover_storage_path, cm.storage_bucket AS cover_storage_bucket
FROM tags t
LEFT JOIN media lm ON lm.id = t.logo_media_id
LEFT JOIN media cm ON cm.id = t.cover_media_id
WHERE t.id = @id;

-- name: CountContentByTag :one
-- Used by DeleteTag guard: spec says "delete tag (only if no content tagged)".
SELECT COUNT(*)::int AS count
FROM content_tags
WHERE tag_id = @tag_id;

-- name: AdminListTags :many
-- Admin list with offset pagination + search (Section 08: GET /admin/tags).
SELECT t.id, t.name, t.slug, t.description, t.rules, t.cover_media_id,
 t.logo_media_id, t.color, t.follower_count, t.content_count, t.is_system,
 t.field_values, t.created_at, t.updated_at,
 lm.storage_path AS logo_storage_path, lm.storage_bucket AS logo_storage_bucket,
 cm.storage_path AS cover_storage_path, cm.storage_bucket AS cover_storage_bucket
FROM tags t
LEFT JOIN media lm ON lm.id = t.logo_media_id
LEFT JOIN media cm ON cm.id = t.cover_media_id
WHERE (@q::text = '' OR t.name ILIKE '%' || @q || '%' OR t.slug ILIKE '%' || @q || '%')
 AND (@min_content_count::int = 0 OR t.content_count >= @min_content_count)
ORDER BY
 CASE WHEN @sort::text = 'popular' THEN t.follower_count END DESC,
 CASE WHEN @sort::text = 'followers' THEN t.follower_count END DESC,
 CASE WHEN @sort::text = 'content' THEN t.content_count END DESC,
 CASE WHEN @sort::text != 'popular' AND @sort::text != 'followers' AND @sort::text != 'content' THEN t.created_at END DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountTags :one
SELECT COUNT(*)::int AS count
FROM tags
WHERE (@q::text = '' OR name ILIKE '%' || @q || '%' OR slug ILIKE '%' || @q || '%')
 AND (@min_content_count::int = 0 OR content_count >= @min_content_count);

-- Admin CRUD (catalog: POST/PATCH/DELETE /admin/tags).

-- name: CreateTag :one
INSERT INTO tags (name, slug, description, rules, cover_media_id, color, is_system, logo_media_id, field_values) VALUES (@name, @slug, @description, @rules, @cover_media_id, @color, @is_system, @logo_media_id, @field_values)

RETURNING id, name, slug, description, rules, cover_media_id, logo_media_id, color,
 follower_count, content_count, is_system, field_values, created_at, updated_at;

-- name: UpdateTag :one
UPDATE tags
SET name = @name, slug = @slug, description = @description, rules = @rules, cover_media_id = @cover_media_id,
 color = @color, logo_media_id = @logo_media_id, is_system = @is_system, field_values = @field_values, updated_at = now()
WHERE id = @id
RETURNING id, name, slug, description, rules, cover_media_id, logo_media_id, color,
 follower_count, content_count, is_system, field_values, created_at, updated_at;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = @id;

-- =====================
-- Tag moderators (Section 49)
-- =====================

-- name: ListTagModerators :many
-- Lists the moderators of a tag, joined to users for display.
SELECT tm.user_id, COALESCE(u.public_username, u.username, '') AS username, tm.created_at,
 COALESCE(up.display_name, '') AS display_name,
 COALESCE(up.avatar_url, '') AS avatar_url
FROM tag_moderators tm
JOIN users u ON u.id = tm.user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE tm.tag_id = @tag_id
ORDER BY tm.created_at ASC;

-- name: ListFirstTagModerators :many
-- Returns the earliest-assigned moderator per tag for a set of tag IDs.
-- Used to project tag_moderators into the legacy single-moderator wire shape
-- (forum_tags.moderator_user_id) — Section 08/49.
SELECT DISTINCT ON (tm.tag_id) tm.tag_id, tm.user_id
FROM tag_moderators tm
WHERE tm.tag_id = ANY(@tag_ids::uuid[])
ORDER BY tm.tag_id, tm.created_at ASC;

-- name: AddTagModerator :one
-- Idempotent: re-adding an existing moderator just refreshes created_at.
INSERT INTO tag_moderators (tag_id, user_id, assigned_by, assigned_by_user_id) VALUES (@tag_id, @user_id, @assigned_by, @assigned_by_user_id)

ON CONFLICT (tag_id, user_id) DO UPDATE SET created_at = now()
RETURNING id, tag_id, user_id, assigned_by, assigned_by_user_id, created_at;

-- name: RemoveTagModerator :exec
DELETE FROM tag_moderators
WHERE tag_id = @tag_id AND user_id = @user_id;
