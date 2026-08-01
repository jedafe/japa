-- Sidebars — Sidebar Management System (Section 59, ADR-066)
-- Dedicated tables for per-app, per-page-pattern, per-usergroup sidebar management.
-- Sidebar widgets are NOT pagebuilder blocks — they use a separate widget type
-- registry (search, theme_toggle, trending_tags, suggested_spaces, ad_slot, etc.).

-- =====================
-- Sidebars CRUD
-- =====================

-- name: ListSidebars :many
SELECT id, tenant_id, app, position, slug, name, page_pattern, visibility,
       required_group_ids, status, created_at, updated_at
FROM sidebars
WHERE tenant_id = @tenant_id
  AND (@app::text = '' OR app = @app)
  AND (@position::text = '' OR position = @position)
  AND (@q::text = '' OR name ILIKE '%' || @q || '%' OR slug ILIKE '%' || @q || '%')
ORDER BY app ASC, position ASC, created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountSidebars :one
SELECT COUNT(*)::int AS count
FROM sidebars
WHERE tenant_id = @tenant_id
  AND (@app::text = '' OR app = @app)
  AND (@position::text = '' OR position = @position)
  AND (@q::text = '' OR name ILIKE '%' || @q || '%' OR slug ILIKE '%' || @q || '%');

-- name: GetSidebarByID :one
SELECT id, tenant_id, app, position, slug, name, page_pattern, visibility,
       required_group_ids, status, created_at, updated_at
FROM sidebars
WHERE id = @id AND tenant_id = @tenant_id;

-- name: CreateSidebar :one
INSERT INTO sidebars (tenant_id, app, position, slug, name, page_pattern,
    visibility, required_group_ids, status)
VALUES (@tenant_id, @app, @position, @slug, @name, @page_pattern,
    @visibility, @required_group_ids, @status)
RETURNING id, tenant_id, app, position, slug, name, page_pattern, visibility,
    required_group_ids, status, created_at, updated_at;

-- name: UpdateSidebar :one
UPDATE sidebars
SET name = @name, page_pattern = @page_pattern, visibility = @visibility,
    required_group_ids = @required_group_ids, status = @status,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, app, position, slug, name, page_pattern, visibility,
    required_group_ids, status, created_at, updated_at;

-- name: DeleteSidebar :exec
DELETE FROM sidebars
WHERE id = @id AND tenant_id = @tenant_id;

-- =====================
-- Sidebar Widgets CRUD
-- =====================

-- name: ListSidebarWidgets :many
SELECT id, tenant_id, sidebar_id, widget_type, label, props, sort_order,
       visibility, required_group_ids, permission_node, status,
       created_at, updated_at
FROM sidebar_widgets
WHERE tenant_id = @tenant_id AND sidebar_id = @sidebar_id
ORDER BY sort_order ASC, created_at ASC;

-- name: GetSidebarWidget :one
SELECT id, tenant_id, sidebar_id, widget_type, label, props, sort_order,
       visibility, required_group_ids, permission_node, status,
       created_at, updated_at
FROM sidebar_widgets
WHERE id = @id AND tenant_id = @tenant_id;

-- name: CreateSidebarWidget :one
INSERT INTO sidebar_widgets (tenant_id, sidebar_id, widget_type, label, props,
    sort_order, visibility, required_group_ids, permission_node, status)
VALUES (@tenant_id, @sidebar_id, @widget_type, @label, @props,
    @sort_order, @visibility, @required_group_ids, sqlc.narg(permission_node),
    @status)
RETURNING id, tenant_id, sidebar_id, widget_type, label, props, sort_order,
    visibility, required_group_ids, permission_node, status,
    created_at, updated_at;

-- name: UpdateSidebarWidget :one
UPDATE sidebar_widgets
SET label = @label, widget_type = @widget_type, props = @props,
    sort_order = @sort_order, visibility = @visibility,
    required_group_ids = @required_group_ids,
    permission_node = sqlc.narg(permission_node), status = @status,
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, sidebar_id, widget_type, label, props, sort_order,
    visibility, required_group_ids, permission_node, status,
    created_at, updated_at;

-- name: DeleteSidebarWidget :exec
DELETE FROM sidebar_widgets
WHERE id = @id AND tenant_id = @tenant_id;

-- name: BulkUpdateSidebarWidgetSortOrder :exec
UPDATE sidebar_widgets
SET sort_order = u.sort_order, updated_at = now()
FROM (
    SELECT unnest(@ids::uuid[]) AS id, unnest(@sort_orders::int[]) AS sort_order
) AS u
WHERE sidebar_widgets.id = u.id AND sidebar_widgets.tenant_id = @tenant_id;

-- =====================
-- Public Resolution
-- =====================

-- name: ListActiveSidebarsForResolve :many
SELECT id, tenant_id, app, position, slug, name, page_pattern, visibility,
       required_group_ids, status, created_at, updated_at
FROM sidebars
WHERE tenant_id = @tenant_id
  AND app = @app
  AND position = @position
  AND status = 'active'
ORDER BY created_at ASC;

-- name: ListActiveSidebarWidgets :many
SELECT id, tenant_id, sidebar_id, widget_type, label, props, sort_order,
       visibility, required_group_ids, permission_node, status,
       created_at, updated_at
FROM sidebar_widgets
WHERE tenant_id = @tenant_id AND sidebar_id = @sidebar_id
  AND status = 'active'
ORDER BY sort_order ASC, created_at ASC;
