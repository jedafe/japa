-- Menus — Navigation menus with hierarchical RBAC-gated items (ADR-054)

-- =====================
-- Menus CRUD
-- =====================

-- name: ListMenus :many
SELECT id, tenant_id, slug, name, placement, status, created_at, updated_at
FROM menus
WHERE tenant_id = @tenant_id
ORDER BY created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMenus :one
SELECT COUNT(*)::int AS count
FROM menus
WHERE tenant_id = @tenant_id;

-- name: GetMenuByID :one
SELECT id, tenant_id, slug, name, placement, status, created_at, updated_at
FROM menus
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetMenuBySlug :one
SELECT id, tenant_id, slug, name, placement, status, created_at, updated_at
FROM menus
WHERE tenant_id = @tenant_id AND slug = @slug;

-- name: CreateMenu :one
INSERT INTO menus (tenant_id, slug, name, placement)
VALUES (@tenant_id, @slug, @name, @placement)
RETURNING id, tenant_id, slug, name, placement, status, created_at, updated_at;

-- name: UpdateMenu :one
UPDATE menus
SET name = @name, placement = @placement, status = @status, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, slug, name, placement, status, created_at, updated_at;

-- name: DeleteMenu :exec
DELETE FROM menus
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Menu Items CRUD
-- =====================

-- name: ListMenuItems :many
SELECT mi.id, mi.tenant_id, mi.menu_id, mi.parent_id, mi.label, mi.url,
       mi.icon, mi.sort_order, mi.permission_node, mi.visibility, mi.target,
       mi.is_ad_slot, mi.ad_slot_config, mi.status, mi.created_at, mi.updated_at
FROM menu_items mi
WHERE mi.tenant_id = @tenant_id AND mi.menu_id = @menu_id
ORDER BY mi.sort_order ASC, mi.created_at ASC;

-- name: GetMenuItem :one
SELECT id, tenant_id, menu_id, parent_id, label, url, icon, sort_order,
       permission_node, visibility, target, is_ad_slot, ad_slot_config,
       status, created_at, updated_at
FROM menu_items
WHERE id = @id AND tenant_id = @tenant_id;

-- name: CreateMenuItem :one
INSERT INTO menu_items (tenant_id, menu_id, parent_id, label, url, icon,
    sort_order, permission_node, visibility, target, is_ad_slot, ad_slot_config)
VALUES (@tenant_id, @menu_id, sqlc.narg(parent_id), @label, @url, sqlc.narg(icon),
    @sort_order, sqlc.narg(permission_node), @visibility, @target, @is_ad_slot,
    sqlc.narg(ad_slot_config))
RETURNING id, tenant_id, menu_id, parent_id, label, url, icon, sort_order,
    permission_node, visibility, target, is_ad_slot, ad_slot_config,
    status, created_at, updated_at;

-- name: UpdateMenuItem :one
UPDATE menu_items
SET label = @label, url = @url, icon = sqlc.narg(icon),
    parent_id = sqlc.narg(parent_id), sort_order = @sort_order,
    permission_node = sqlc.narg(permission_node), visibility = @visibility,
    target = @target, is_ad_slot = @is_ad_slot,
    ad_slot_config = sqlc.narg(ad_slot_config), status = @status,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, menu_id, parent_id, label, url, icon, sort_order,
    permission_node, visibility, target, is_ad_slot, ad_slot_config,
    status, created_at, updated_at;

-- name: DeleteMenuItem :exec
DELETE FROM menu_items
WHERE id = @id AND tenant_id = @tenant_id;

-- name: BulkUpdateMenuItemSortOrder :exec
UPDATE menu_items
SET sort_order = u.sort_order, updated_at = now()
FROM (
    SELECT unnest(@ids::uuid[]) AS id, unnest(@sort_orders::int[]) AS sort_order
) AS u
WHERE menu_items.id = u.id AND menu_items.tenant_id = @tenant_id;

-- =====================
-- Public resolution (no tenant filter — called from public endpoint)
-- =====================

-- name: GetMenuBySlugForTenant :one
SELECT id, tenant_id, slug, name, placement, status, created_at, updated_at
FROM menus
WHERE tenant_id = @tenant_id AND slug = @slug AND status = 'active';
