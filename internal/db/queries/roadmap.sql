-- Public product roadmap (§113). Items, categories, votes.
-- Ref: japaconnect-engineering/sections/113-public-product-roadmap.md

-- =====================
-- Roadmap items
-- =====================

-- name: CreateRoadmapItem :one
INSERT INTO roadmap_items (
    title, slug, summary, body, category_id, status, sort_order,
    is_flagship, target_window, feature_flag_key, content_id,
    submitted_by, approved_by
) VALUES (
    @title, @slug, sqlc.narg(summary), sqlc.narg(body),
    sqlc.narg(category_id), @status, @sort_order,
    @is_flagship, sqlc.narg(target_window), sqlc.narg(feature_flag_key),
    sqlc.narg(content_id), sqlc.narg(submitted_by), sqlc.narg(approved_by)
) RETURNING *;

-- name: GetRoadmapItemByID :one
SELECT * FROM roadmap_items WHERE id = @id;

-- name: GetRoadmapItemIDBySlug :one
-- Existence check ignoring soft deletes — the slug UNIQUE constraint covers
-- deleted rows too, so uniqueItemSlug must see them or create hits 23505.
SELECT id FROM roadmap_items WHERE slug = @slug;

-- name: GetRoadmapItemBySlug :one
SELECT i.*, c.name AS category_name, c.slug AS category_slug, c.color AS category_color,
       (v.user_id IS NOT NULL)::bool AS has_voted
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id AND c.deleted_at IS NULL
LEFT JOIN roadmap_votes v ON v.item_id = i.id AND v.user_id = sqlc.narg(viewer_id)
WHERE i.slug = @slug AND i.deleted_at IS NULL;

-- name: ListPublicRoadmapItems :many
SELECT i.*, c.name AS category_name, c.slug AS category_slug, c.color AS category_color,
       (v.user_id IS NOT NULL)::bool AS has_voted
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id AND c.deleted_at IS NULL
LEFT JOIN roadmap_votes v ON v.item_id = i.id AND v.user_id = sqlc.narg(viewer_id)
WHERE i.status = ANY(@statuses::text[]) AND i.deleted_at IS NULL
  AND (sqlc.narg(category_slug)::text = '' OR c.slug = sqlc.narg(category_slug)::text)
  AND (i.status <> 'shipped' OR i.shipped_at > now() - make_interval(days => @shipped_window_days))
ORDER BY i.is_flagship DESC, i.sort_order ASC, i.vote_count DESC, i.created_at DESC;

-- name: ListRoadmapChangelog :many
SELECT i.*, c.name AS category_name, c.slug AS category_slug, c.color AS category_color
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id AND c.deleted_at IS NULL
WHERE i.status = 'shipped' AND i.deleted_at IS NULL
ORDER BY i.shipped_at DESC;

-- name: ListRoadmapRelatedItems :many
-- Lightweight related-items lookup for the item detail page: public items
-- in the same category, excluding the item itself, capped at 3. Replaces
-- the full board refetch + client-side filter the detail route used before.
SELECT i.*, c.name AS category_name, c.slug AS category_slug, c.color AS category_color,
       (v.user_id IS NOT NULL)::bool AS has_voted
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id AND c.deleted_at IS NULL
LEFT JOIN roadmap_votes v ON v.item_id = i.id AND v.user_id = sqlc.narg(viewer_id)
WHERE i.status = ANY(@statuses::text[]) AND i.deleted_at IS NULL
  AND i.id <> @exclude_id
  AND i.category_id = @category_id
ORDER BY i.is_flagship DESC, i.sort_order ASC, i.vote_count DESC, i.created_at DESC
LIMIT 3;

-- name: ListAdminRoadmapItems :many
SELECT i.*, c.name AS category_name
FROM roadmap_items i
LEFT JOIN roadmap_categories c ON c.id = i.category_id
WHERE (sqlc.narg(status)::text = '' OR i.status = sqlc.narg(status)::text)
  AND (sqlc.narg(category_id)::uuid IS NULL OR i.category_id = sqlc.narg(category_id)::uuid)
  AND (sqlc.narg(search)::text = '' OR i.title ILIKE '%' || sqlc.narg(search)::text || '%' OR i.summary ILIKE '%' || sqlc.narg(search)::text || '%')
  AND (@include_deleted::bool OR i.deleted_at IS NULL)
ORDER BY
  CASE WHEN sqlc.narg(sort)::text = 'votes' THEN i.vote_count END DESC NULLS LAST,
  CASE WHEN sqlc.narg(sort)::text = 'title' THEN i.title END ASC NULLS LAST,
  CASE WHEN sqlc.narg(sort)::text IS NULL OR sqlc.narg(sort)::text = '' OR sqlc.narg(sort)::text = 'recent' THEN i.created_at END DESC NULLS LAST
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAdminRoadmapItems :one
SELECT count(*) FROM roadmap_items i
WHERE (sqlc.narg(status)::text = '' OR i.status = sqlc.narg(status)::text)
  AND (sqlc.narg(category_id)::uuid IS NULL OR i.category_id = sqlc.narg(category_id)::uuid)
  AND (sqlc.narg(search)::text = '' OR i.title ILIKE '%' || sqlc.narg(search)::text || '%' OR i.summary ILIKE '%' || sqlc.narg(search)::text || '%')
  AND (@include_deleted::bool OR i.deleted_at IS NULL);

-- name: UpdateRoadmapItem :one
UPDATE roadmap_items SET
    title = @title, slug = @slug,
    summary = sqlc.narg(summary), body = sqlc.narg(body),
    category_id = sqlc.narg(category_id),
    sort_order = @sort_order, is_flagship = @is_flagship,
    target_window = sqlc.narg(target_window),
    feature_flag_key = sqlc.narg(feature_flag_key),
    approved_by = sqlc.narg(approved_by),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: UpdateRoadmapItemStatus :one
UPDATE roadmap_items SET
    status = @status,
    shipped_at = CASE WHEN @status = 'shipped' AND shipped_at IS NULL THEN now() ELSE shipped_at END,
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SetRoadmapItemContent :exec
UPDATE roadmap_items SET content_id = @content_id, updated_at = now() WHERE id = @id;

-- name: SoftDeleteRoadmapItem :exec
UPDATE roadmap_items SET deleted_at = now(), updated_at = now() WHERE id = @id AND deleted_at IS NULL;

-- name: RestoreRoadmapItem :exec
UPDATE roadmap_items SET deleted_at = NULL, updated_at = now() WHERE id = @id AND deleted_at IS NOT NULL;

-- name: HardDeleteRoadmapItem :exec
DELETE FROM roadmap_items WHERE id = @id AND deleted_at IS NOT NULL;

-- name: UpdateRoadmapShadowContent :exec
UPDATE contents SET title = @title, body = @body, content_hash = sqlc.narg(content_hash), updated_at = now()
WHERE id = @content_id AND content_type = 'roadmap';

-- name: SoftDeleteRoadmapShadowContent :exec
-- The shadow row tracks the item's lifecycle — a soft-deleted item must not
-- leave its §49 comments thread publicly readable.
UPDATE contents SET deleted_at = now(), updated_at = now()
WHERE id = @content_id AND content_type = 'roadmap' AND deleted_at IS NULL;

-- name: RestoreRoadmapShadowContent :exec
UPDATE contents SET deleted_at = NULL, updated_at = now()
WHERE id = @content_id AND content_type = 'roadmap' AND deleted_at IS NOT NULL;

-- name: HardDeleteRoadmapShadowContent :exec
-- Permanent removal under the admin.hard_delete ?force=true path (Rule 11);
-- comments on the shadow row cascade via comments_content_id_fkey.
DELETE FROM contents WHERE id = @content_id AND content_type = 'roadmap';

-- name: ReorderRoadmapItems :exec
UPDATE roadmap_items SET sort_order = v.ord
FROM (SELECT unnest(@ids::uuid[]) AS id, unnest(@orders::int[]) AS ord) v
WHERE roadmap_items.id = v.id;

-- =====================
-- Roadmap categories
-- =====================

-- name: ListRoadmapCategories :many
SELECT * FROM roadmap_categories
WHERE deleted_at IS NULL
ORDER BY sort_order ASC, name ASC;

-- name: ListAdminRoadmapCategories :many
SELECT * FROM roadmap_categories
ORDER BY sort_order ASC, name ASC;

-- name: GetRoadmapCategoryByID :one
SELECT * FROM roadmap_categories WHERE id = @id;

-- name: GetRoadmapCategoryIDBySlug :one
-- Existence check ignoring soft deletes — the slug UNIQUE constraint covers
-- deleted rows too, so admin create/update must see them to return 409
-- instead of a raw 23505.
SELECT id FROM roadmap_categories WHERE slug = @slug;

-- name: CreateRoadmapCategory :one
INSERT INTO roadmap_categories (slug, name, description, color, sort_order)
VALUES (@slug, @name, sqlc.narg(description), sqlc.narg(color), @sort_order)
RETURNING *;

-- name: UpdateRoadmapCategory :one
UPDATE roadmap_categories SET
    slug = @slug, name = @name,
    description = sqlc.narg(description), color = sqlc.narg(color),
    sort_order = @sort_order, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SoftDeleteRoadmapCategory :exec
UPDATE roadmap_categories SET deleted_at = now(), updated_at = now() WHERE id = @id AND deleted_at IS NULL;

-- =====================
-- Roadmap votes
-- =====================

-- name: TryCreateRoadmapVote :one
INSERT INTO roadmap_votes (item_id, user_id) VALUES (@item_id, @user_id)
ON CONFLICT (item_id, user_id) DO NOTHING
RETURNING item_id;

-- name: DeleteRoadmapVote :execrows
DELETE FROM roadmap_votes WHERE item_id = @item_id AND user_id = @user_id;

-- name: SyncRoadmapVoteCount :exec
UPDATE roadmap_items SET vote_count = (
    SELECT count(*) FROM roadmap_votes v WHERE v.item_id = @item_id
) WHERE id = @item_id;

-- name: ListRoadmapVoterIDs :many
SELECT user_id FROM roadmap_votes WHERE item_id = @item_id;

-- =====================
-- Sitemap + shadow contents row
-- =====================

-- name: ListRoadmapSitemapItems :many
SELECT slug, updated_at FROM roadmap_items
WHERE status IN ('under_review','planned','in_progress','testing','shipped') AND deleted_at IS NULL
ORDER BY updated_at DESC;

-- =====================
-- Feature-flag linkage (unified rollout §4.2–§4.4)
-- =====================

-- name: GetRoadmapItemByFlagKey :one
-- The item linked to a feature flag — the lookup behind SyncFlagPhase.
SELECT * FROM roadmap_items WHERE feature_flag_key = @flag_key AND deleted_at IS NULL;

-- name: GetOtherRoadmapItemByFlagKey :one
-- Friendly pre-check behind the 000808 partial unique index: is another
-- live item already linked to this flag? exclude_id is the item being
-- edited so a PATCH re-setting the same key does not trip; pass the zero
-- UUID on create.
SELECT id FROM roadmap_items
WHERE feature_flag_key = @flag_key AND deleted_at IS NULL AND id <> @exclude_id;

-- name: ListRoadmapItemsByFlagKeys :many
-- Batch variant for the control-panel extensions list (one query for the
-- whole catalog, not N lookups).
SELECT * FROM roadmap_items WHERE feature_flag_key = ANY(@flag_keys::text[]) AND deleted_at IS NULL;

-- name: ClearRoadmapItemFlagKey :exec
-- Flag deleted → unlink every linked item; items keep their last status
-- and return to the manual lifecycle (§4.3).
UPDATE roadmap_items SET feature_flag_key = NULL, updated_at = now() WHERE feature_flag_key = @flag_key;

-- name: GetFeatureFlagRowByKey :one
-- Flag row existence + config read for link validation, delete detection,
-- and apply-current-phase on link. feature_flags lives outside the
-- roadmap tables but is queried here to keep flag reads consistent.
SELECT key, enabled, config FROM feature_flags WHERE key = @key;

-- name: GetRoadmapShadowAuthorID :one
-- Fallback author for shadow contents rows created by automated flag sync
-- (no acting admin): the oldest account, mirroring the 000798 backfill.
SELECT id FROM users ORDER BY created_at ASC LIMIT 1;

-- name: CreateRoadmapShadowContent :one
INSERT INTO contents (
    author_id, content_type, title, slug, body, body_html,
    field_values, content_hash, seo_indexable, status, visibility,
    read_time_minutes, published_at
) VALUES (
    @author_id, 'roadmap', @title, @slug, @body, '',
    '{}'::jsonb, sqlc.narg(content_hash), false, 'published', 'public',
    1, now()
) RETURNING id;
