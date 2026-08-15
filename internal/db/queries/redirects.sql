-- Redirects — Admin-managed URL redirect rules (ADR-056)

-- name: ListRedirects :many
SELECT id, tenant_id, source_path, target_url, http_status, is_active,
       notes, hit_count, last_hit_at, created_at, updated_at
FROM redirects
WHERE tenant_id = @tenant_id
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
  AND (@search::text = '' OR source_path ILIKE '%' || @search || '%' OR target_url ILIKE '%' || @search || '%')
ORDER BY
  -- Dynamic sort: @sort is the column name, @dir is 'asc' or 'desc'.
  -- Fallback to created_at DESC when @sort is empty or unrecognized.
  CASE WHEN @sort::text = 'source_path' AND @dir::text = 'asc'  THEN source_path END ASC,
  CASE WHEN @sort::text = 'source_path' AND @dir::text = 'desc' THEN source_path END DESC,
  CASE WHEN @sort::text = 'target_url'  AND @dir::text = 'asc'  THEN target_url  END ASC,
  CASE WHEN @sort::text = 'target_url'  AND @dir::text = 'desc' THEN target_url  END DESC,
  CASE WHEN @sort::text = 'http_status' AND @dir::text = 'asc'  THEN http_status END ASC,
  CASE WHEN @sort::text = 'http_status' AND @dir::text = 'desc' THEN http_status END DESC,
  CASE WHEN @sort::text = 'is_active'   AND @dir::text = 'asc'  THEN is_active   END ASC,
  CASE WHEN @sort::text = 'is_active'   AND @dir::text = 'desc' THEN is_active   END DESC,
  CASE WHEN @sort::text = 'hit_count'   AND @dir::text = 'asc'  THEN hit_count   END ASC,
  CASE WHEN @sort::text = 'hit_count'   AND @dir::text = 'desc' THEN hit_count   END DESC,
  CASE WHEN @sort::text = 'created_at'  AND @dir::text = 'asc'  THEN created_at  END ASC,
  CASE WHEN @sort::text = 'created_at'  AND @dir::text = 'desc' THEN created_at  END DESC,
  CASE WHEN @sort::text = 'updated_at'  AND @dir::text = 'asc'  THEN updated_at  END ASC,
  CASE WHEN @sort::text = 'updated_at'  AND @dir::text = 'desc' THEN updated_at  END DESC,
  CASE WHEN @sort::text = '' OR @sort::text NOT IN ('source_path','target_url','http_status','is_active','hit_count','created_at','updated_at') THEN created_at END DESC
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
