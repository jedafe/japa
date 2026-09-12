-- name: ListMenus :many
-- Admin list with optional q (name/slug ILIKE), placement, status filters
-- and sort. Sort keys: name, slug, placement, status, updated_at (default
-- created_at ASC). Sort direction: asc/desc (default asc). Ref: ADR-054
-- admin list "must filter + sort + paginate". Includes item_count (correlated
-- subquery) so the admin list can show how many items each menu has without
-- an N+1 fetch.
SELECT m.id, m.slug, m.name, m.placement, m.status, m.created_at, m.updated_at,
 (SELECT COUNT(*)::int FROM menu_items mi WHERE mi.menu_id = m.id) AS item_count
FROM menus m
WHERE (@q::text = '' OR m.name ILIKE '%' || @q || '%' OR m.slug ILIKE '%' || @q || '%')
 AND (@placement::text = '' OR m.placement = @placement)
 AND (@status::text = '' OR m.status = @status)
ORDER BY
 CASE WHEN @sort_dir::text = 'asc' AND @sort::text = 'name' THEN m.name END ASC,
 CASE WHEN @sort_dir::text = 'asc' AND @sort::text = 'slug' THEN m.slug END ASC,
 CASE WHEN @sort_dir::text = 'asc' AND @sort::text = 'placement' THEN m.placement END ASC,
 CASE WHEN @sort_dir::text = 'asc' AND @sort::text = 'status' THEN m.status END ASC,
 CASE WHEN @sort_dir::text = 'asc' AND @sort::text = 'updated_at' THEN m.updated_at END ASC,
 CASE WHEN @sort_dir::text = 'desc' AND @sort::text = 'name' THEN m.name END DESC,
 CASE WHEN @sort_dir::text = 'desc' AND @sort::text = 'slug' THEN m.slug END DESC,
 CASE WHEN @sort_dir::text = 'desc' AND @sort::text = 'placement' THEN m.placement END DESC,
 CASE WHEN @sort_dir::text = 'desc' AND @sort::text = 'status' THEN m.status END DESC,
 CASE WHEN @sort_dir::text = 'desc' AND @sort::text = 'updated_at' THEN m.updated_at END DESC,
 m.created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMenus :one
SELECT COUNT(*)::int AS count
FROM menus
WHERE (@q::text = '' OR name ILIKE '%' || @q || '%' OR slug ILIKE '%' || @q || '%')
 AND (@placement::text = '' OR placement = @placement)
 AND (@status::text = '' OR status = @status);

-- name: GetMenuByID :one
SELECT id, tenant_id, slug, name, placement, status, created_at, updated_at
FROM menus
WHERE id = @id;

-- name: GetMenuBySlug :one
SELECT id, tenant_id, slug, name, placement, status, created_at, updated_at
FROM menus
WHERE slug = @slug;

-- name: CreateMenu :one
INSERT INTO menus (slug, name, placement, status) VALUES (@slug, @name, @placement, @status)

RETURNING id, tenant_id, slug, name, placement, status, created_at, updated_at;

-- name: UpdateMenu :one
UPDATE menus
SET name = @name, placement = @placement, status = @status, updated_at = now()
WHERE id = @id
RETURNING id, tenant_id, slug, name, placement, status, created_at, updated_at;

-- name: DeleteMenu :exec
DELETE FROM menus
WHERE id = @id;

-- =====================
-- Menu Items CRUD
-- =====================

-- name: ListMenuItems :many
SELECT mi.id, mi.menu_id, mi.parent_id, mi.label, mi.url,
 mi.icon, mi.sort_order, mi.permission_node, mi.visibility, mi.target,
 mi.is_ad_slot, mi.ad_slot_config, mi.status, mi.required_group_ids,
 mi.feature_flag, mi.created_at, mi.updated_at
FROM menu_items mi
WHERE mi.menu_id = @menu_id
ORDER BY mi.sort_order ASC, mi.created_at ASC;

-- name: GetMenuItem :one
SELECT id, menu_id, parent_id, label, url, icon, sort_order,
 permission_node, visibility, target, is_ad_slot, ad_slot_config,
 status, required_group_ids, feature_flag, created_at, updated_at
FROM menu_items
WHERE id = @id;

-- name: CreateMenuItem :one
INSERT INTO menu_items (menu_id, parent_id, label, url, icon, sort_order, permission_node, visibility, target, is_ad_slot, ad_slot_config, required_group_ids, feature_flag, status) VALUES (@menu_id, sqlc.narg(parent_id), @label, @url, sqlc.narg(icon), @sort_order, sqlc.narg(permission_node), @visibility, @target, @is_ad_slot, sqlc.narg(ad_slot_config), @required_group_ids, sqlc.narg(feature_flag), @status)

RETURNING id, menu_id, parent_id, label, url, icon, sort_order,
 permission_node, visibility, target, is_ad_slot, ad_slot_config,
 status, required_group_ids, feature_flag, created_at, updated_at;

-- name: UpdateMenuItem :one
UPDATE menu_items
SET label = @label, url = @url, icon = sqlc.narg(icon),
 parent_id = sqlc.narg(parent_id), sort_order = @sort_order,
 permission_node = sqlc.narg(permission_node), visibility = @visibility,
 target = @target, is_ad_slot = @is_ad_slot,
 ad_slot_config = sqlc.narg(ad_slot_config), status = @status,
 required_group_ids = @required_group_ids,
 feature_flag = sqlc.narg(feature_flag),
 updated_at = now()
WHERE id = @id
RETURNING id, menu_id, parent_id, label, url, icon, sort_order,
 permission_node, visibility, target, is_ad_slot, ad_slot_config,
 status, required_group_ids, feature_flag, created_at, updated_at;

-- name: DeleteMenuItem :exec
DELETE FROM menu_items
WHERE id = @id;

-- name: BulkUpdateMenuItemSortOrder :exec
UPDATE menu_items
SET sort_order = u.sort_order, updated_at = now()
FROM (
 SELECT unnest(@ids::uuid[]) AS id, unnest(@sort_orders::int[]) AS sort_order
) AS u
WHERE menu_items.id = u.id;

-- =====================
-- Public resolution (no tenant filter — called from public endpoint)
-- =====================

-- name: GetMenuBySlugForTenant :one
SELECT id, tenant_id, slug, name, placement, status, created_at, updated_at
FROM menus
WHERE slug = @slug AND status = 'active';

-- name: GetUserGroupIDs :many
SELECT group_id::text AS group_id
FROM user_group_memberships
WHERE user_id = @user_id;
