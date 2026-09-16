-- name: ListTemplateLiterals :many
-- Returns all template literals for a platform with optional search and active filter.
SELECT id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at
FROM template_literals
WHERE deleted_at IS NULL
 AND (@search::text = '' OR key ILIKE '%' || @search || '%' OR label ILIKE '%' || @search || '%')
 AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
 AND (@kind::text = '' OR kind = @kind::text)
ORDER BY
 CASE WHEN @sort::text = 'key' AND @dir::text = 'asc' THEN key END ASC,
 CASE WHEN @sort::text = 'key' AND @dir::text = 'desc' THEN key END DESC,
 CASE WHEN @sort::text = 'created_at' AND @dir::text = 'asc' THEN created_at END ASC,
 CASE WHEN @sort::text = 'created_at' AND @dir::text = 'desc' THEN created_at END DESC,
 CASE WHEN @sort::text = '' OR @sort::text NOT IN ('key', 'created_at') THEN created_at END DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountTemplateLiterals :one
SELECT COUNT(*) FROM template_literals
WHERE deleted_at IS NULL
 AND (@search::text = '' OR key ILIKE '%' || @search || '%' OR label ILIKE '%' || @search || '%')
 AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
 AND (@kind::text = '' OR kind = @kind::text);

-- name: GetTemplateLiteral :one
SELECT id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at
FROM template_literals
WHERE id = @id
 AND deleted_at IS NULL;

-- name: GetTemplateLiteralByKey :one
SELECT id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at
FROM template_literals
WHERE key = @key
 AND deleted_at IS NULL;

-- name: CreateTemplateLiteral :one
INSERT INTO template_literals (key, kind, value, fallback, label, description, category, is_active) VALUES (@key, @kind, @value, @fallback, @label, @description, @category, @is_active)

RETURNING id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at;

-- name: UpdateTemplateLiteral :one
UPDATE template_literals
SET kind = @kind,
 value = @value,
 fallback = @fallback,
 label = @label,
 description = @description,
 category = @category,
 is_active = @is_active,
 updated_at = now()
WHERE id = @id
 AND deleted_at IS NULL
RETURNING id, key, kind, value, fallback, label, description, category, is_system, is_active, created_at, updated_at, deleted_at;

-- name: DeleteTemplateLiteral :exec
UPDATE template_literals
SET deleted_at = now(), updated_at = now()
WHERE id = @id
 AND deleted_at IS NULL;
