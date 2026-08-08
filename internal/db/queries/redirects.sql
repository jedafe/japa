-- Redirects — Admin-managed URL redirect rules (ADR-054)

-- name: ListRedirects :many
SELECT id, tenant_id, source_path, target_url, http_status, is_active,
       notes, hit_count, last_hit_at, created_at, updated_at
FROM redirects
WHERE tenant_id = @tenant_id
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
  AND (@search::text = '' OR source_path ILIKE '%' || @search || '%' OR target_url ILIKE '%' || @search || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountRedirects :one
SELECT COUNT(*)::int AS count
FROM redirects
WHERE tenant_id = @tenant_id
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
  AND (@search::text = '' OR source_path ILIKE '%' || @search || '%' OR target_url ILIKE '%' || @search || '%');

-- name: GetRedirect :one
SELECT id, tenant_id, source_path, target_url, http_status, is_active,
       notes, hit_count, last_hit_at, created_at, updated_at
FROM redirects
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetRedirectByPath :one
SELECT id, tenant_id, source_path, target_url, http_status, is_active,
       notes, hit_count, last_hit_at, created_at, updated_at
FROM redirects
WHERE tenant_id = @tenant_id AND source_path = @source_path AND is_active = true;

-- name: CreateRedirect :one
INSERT INTO redirects (tenant_id, source_path, target_url, http_status, notes)
VALUES (@tenant_id, @source_path, @target_url, @http_status, sqlc.narg(notes))
RETURNING id, tenant_id, source_path, target_url, http_status, is_active,
    notes, hit_count, last_hit_at, created_at, updated_at;

-- name: UpdateRedirect :one
UPDATE redirects
SET source_path = @source_path, target_url = @target_url,
    http_status = @http_status, is_active = @is_active,
    notes = sqlc.narg(notes), updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, source_path, target_url, http_status, is_active,
    notes, hit_count, last_hit_at, created_at, updated_at;

-- name: DeleteRedirect :exec
DELETE FROM redirects
WHERE id = @id AND tenant_id = @tenant_id;

-- name: RecordRedirectHit :exec
UPDATE redirects
SET hit_count = hit_count + 1, last_hit_at = now()
WHERE id = @id AND tenant_id = @tenant_id;

-- name: BulkUpdateRedirectStatus :exec
UPDATE redirects
SET is_active = @is_active, updated_at = now()
WHERE tenant_id = @tenant_id AND id = ANY(@ids::uuid[]);

-- name: BulkDeleteRedirects :exec
DELETE FROM redirects
WHERE tenant_id = @tenant_id AND id = ANY(@ids::uuid[]);

-- name: GetRedirectsByIDs :many
SELECT id, tenant_id, source_path, target_url, http_status, is_active,
       notes, hit_count, last_hit_at, created_at, updated_at
FROM redirects
WHERE tenant_id = @tenant_id AND id = ANY(@ids::uuid[]);
