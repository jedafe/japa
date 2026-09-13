-- name: ListSidebars :many
SELECT s.id, s.app, s.position, s.slug, s.name, s.page_pattern, s.visibility,
 s.required_group_ids, s.status, s.created_at, s.updated_at,
 COALESCE(sw.cnt, 0)::int AS widget_count
FROM sidebars s
LEFT JOIN (
 SELECT sidebar_id, COUNT(*)::int AS cnt
 FROM sidebar_widgets
 GROUP BY sidebar_id
) sw ON sw.sidebar_id = s.id
WHERE (@app::text = '' OR s.app = @app)
 AND (@position::text = '' OR s.position = @position)
 AND (@status::text = '' OR s.status = @status)
 AND (@q::text = '' OR s.name ILIKE '%' || @q || '%' OR s.slug ILIKE '%' || @q || '%')
ORDER BY
 CASE WHEN @sort::text = 'name:asc' THEN s.name END ASC,
 CASE WHEN @sort::text = 'name:desc' THEN s.name END DESC,
 CASE WHEN @sort::text = 'app:asc' THEN s.app END ASC,
 CASE WHEN @sort::text = 'app:desc' THEN s.app END DESC,
 CASE WHEN @sort::text = 'position:asc' THEN s.position END ASC,
 CASE WHEN @sort::text = 'position:desc' THEN s.position END DESC,
 CASE WHEN @sort::text = 'slug:asc' THEN s.slug END ASC,
 CASE WHEN @sort::text = 'slug:desc' THEN s.slug END DESC,
 CASE WHEN @sort::text = 'page_pattern:asc' THEN s.page_pattern END ASC,
 CASE WHEN @sort::text = 'page_pattern:desc' THEN s.page_pattern END DESC,
 CASE WHEN @sort::text = 'status:asc' THEN s.status END ASC,
 CASE WHEN @sort::text = 'status:desc' THEN s.status END DESC,
 CASE WHEN @sort::text = 'updated_at:asc' THEN s.updated_at END ASC,
 CASE WHEN @sort::text = 'updated_at:desc' THEN s.updated_at END DESC,
 CASE WHEN @sort::text = 'created_at:asc' THEN s.created_at END ASC,
 CASE WHEN @sort::text = 'created_at:desc' THEN s.created_at END DESC,
 -- Default sort when @sort is empty or unrecognized
 s.app ASC, s.position ASC, s.created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountSidebars :one
SELECT COUNT(*)::int AS count
FROM sidebars
WHERE (@app::text = '' OR app = @app)
 AND (@position::text = '' OR position = @position)
 AND (@status::text = '' OR status = @status)
 AND (@q::text = '' OR name ILIKE '%' || @q || '%' OR slug ILIKE '%' || @q || '%');

-- name: GetSidebarByID :one
SELECT id, app, position, slug, name, page_pattern, visibility,
 required_group_ids, status, created_at, updated_at
FROM sidebars
WHERE id = @id;

-- name: CreateSidebar :one
INSERT INTO sidebars (app, position, slug, name, page_pattern, visibility, required_group_ids, status) VALUES (@app, @position, @slug, @name, @page_pattern, @visibility, @required_group_ids, @status)

RETURNING id, app, position, slug, name, page_pattern, visibility,
 required_group_ids, status, created_at, updated_at;

-- name: UpdateSidebar :one
UPDATE sidebars
SET name = @name, page_pattern = @page_pattern, visibility = @visibility,
 required_group_ids = @required_group_ids, status = @status,
 updated_at = now()
WHERE id = @id
RETURNING id, app, position, slug, name, page_pattern, visibility,
 required_group_ids, status, created_at, updated_at;

-- name: DeleteSidebar :exec
DELETE FROM sidebars
WHERE id = @id;

-- =====================
-- Sidebar Widgets CRUD
-- =====================

-- name: ListSidebarWidgets :many
SELECT id, sidebar_id, widget_type, label, props, sort_order,
 visibility, required_group_ids, permission_node, status,
 created_at, updated_at
FROM sidebar_widgets
WHERE sidebar_id = @sidebar_id
ORDER BY sort_order ASC, created_at ASC;

-- name: GetSidebarWidget :one
SELECT id, sidebar_id, widget_type, label, props, sort_order,
 visibility, required_group_ids, permission_node, status,
 created_at, updated_at
FROM sidebar_widgets
WHERE id = @id;

-- name: CreateSidebarWidget :one
INSERT INTO sidebar_widgets (sidebar_id, widget_type, label, props, sort_order, visibility, required_group_ids, permission_node, status) VALUES (@sidebar_id, @widget_type, @label, @props, @sort_order, @visibility, @required_group_ids, sqlc.narg(permission_node), @status)

RETURNING id, sidebar_id, widget_type, label, props, sort_order,
 visibility, required_group_ids, permission_node, status,
 created_at, updated_at;

-- name: UpdateSidebarWidget :one
UPDATE sidebar_widgets
SET label = @label, widget_type = @widget_type, props = @props,
 sort_order = @sort_order, visibility = @visibility,
 required_group_ids = @required_group_ids,
 permission_node = sqlc.narg(permission_node), status = @status,
 updated_at = now()
WHERE id = @id
RETURNING id, sidebar_id, widget_type, label, props, sort_order,
 visibility, required_group_ids, permission_node, status,
 created_at, updated_at;

-- name: DeleteSidebarWidget :exec
DELETE FROM sidebar_widgets
WHERE id = @id;

-- name: BulkUpdateSidebarWidgetSortOrder :exec
UPDATE sidebar_widgets
SET sort_order = u.sort_order, updated_at = now()
FROM (
 SELECT unnest(@ids::uuid[]) AS id, unnest(@sort_orders::int[]) AS sort_order
) AS u
WHERE sidebar_widgets.id = u.id;

-- =====================
-- Public Resolution
-- =====================

-- name: ListActiveSidebarsForResolve :many
SELECT id, app, position, slug, name, page_pattern, visibility,
 required_group_ids, status, created_at, updated_at
FROM sidebars
WHERE app = @app
 AND position = @position
 AND status = 'active'
ORDER BY created_at ASC;

-- name: ListActiveSidebarWidgets :many
SELECT id, sidebar_id, widget_type, label, props, sort_order,
 visibility, required_group_ids, permission_node, status,
 created_at, updated_at
FROM sidebar_widgets
WHERE sidebar_id = @sidebar_id
 AND status = 'active'
ORDER BY sort_order ASC, created_at ASC;
