-- Video import jobs: bulk YouTube channel/playlist/keyword imports.

-- name: CreateVideoImportJob :one
INSERT INTO video_import_jobs (
    tenant_id, user_id, source_type, source_url, query, target_author_user_id,
    status, total_items, imported_count, failed_count, error_log, auto_publish
) VALUES (
    @tenant_id, @user_id, @source_type, @source_url, @query, @target_author_user_id,
    COALESCE(@status, 'pending'), @total_items, @imported_count, @failed_count,
    COALESCE(@error_log, '[]'::jsonb), @auto_publish
)
RETURNING
    id, tenant_id, user_id, source_type, source_url, query, target_author_user_id,
    status, total_items, imported_count, failed_count, error_log, created_at,
    completed_at, auto_publish, started_at;

-- name: GetVideoImportJobByID :one
SELECT
    id, tenant_id, user_id, source_type, source_url, query, target_author_user_id,
    status, total_items, imported_count, failed_count, error_log, created_at,
    completed_at, auto_publish, started_at
FROM video_import_jobs
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListVideoImportJobsByUser :many
SELECT
    id, tenant_id, user_id, source_type, source_url, query, target_author_user_id,
    status, total_items, imported_count, failed_count, error_log, created_at,
    completed_at, auto_publish, started_at
FROM video_import_jobs
WHERE tenant_id = @tenant_id AND user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateVideoImportJob :one
UPDATE video_import_jobs
SET status = @status,
    total_items = @total_items,
    imported_count = @imported_count,
    failed_count = @failed_count,
    error_log = COALESCE(sqlc.narg(error_log), error_log),
    started_at = COALESCE(sqlc.narg(started_at), started_at),
    completed_at = COALESCE(sqlc.narg(completed_at), completed_at)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING
    id, tenant_id, user_id, source_type, source_url, query, target_author_user_id,
    status, total_items, imported_count, failed_count, error_log, created_at,
    completed_at, auto_publish, started_at;

-- name: ListVideoImportJobsForAdmin :many
-- Admin queue with optional status/source_type filters, text search, and sorting.
SELECT
    vj.id, vj.tenant_id, vj.user_id, vj.source_type, vj.source_url, vj.query,
    vj.target_author_user_id, vj.status, vj.total_items, vj.imported_count,
    vj.failed_count, vj.error_log, vj.created_at, vj.completed_at, vj.auto_publish,
    vj.started_at,
    u.username AS owner_username
FROM video_import_jobs vj
JOIN users u ON u.id = vj.user_id AND u.tenant_id = vj.tenant_id
WHERE vj.tenant_id = @tenant_id
  AND (@status::text = '' OR vj.status = @status)
  AND (@source_type::text = '' OR vj.source_type = @source_type)
  AND (@q::text = '' OR vj.query ILIKE '%' || @q || '%' OR vj.source_url ILIKE '%' || @q || '%' OR u.username ILIKE '%' || @q || '%')
ORDER BY
    CASE WHEN @sort = 'created_at' AND @dir = 'asc' THEN vj.created_at END ASC NULLS LAST,
    CASE WHEN @sort = 'created_at' AND @dir = 'desc' THEN vj.created_at END DESC NULLS FIRST,
    CASE WHEN @sort = 'status' AND @dir = 'asc' THEN vj.status END ASC NULLS LAST,
    CASE WHEN @sort = 'status' AND @dir = 'desc' THEN vj.status END DESC NULLS FIRST,
    vj.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountVideoImportJobsForAdmin :one
-- Total for admin video import jobs list (same filters as ListVideoImportJobsForAdmin).
SELECT COUNT(*)::int AS total
FROM video_import_jobs vj
JOIN users u ON u.id = vj.user_id AND u.tenant_id = vj.tenant_id
WHERE vj.tenant_id = @tenant_id
  AND (@status::text = '' OR vj.status = @status)
  AND (@source_type::text = '' OR vj.source_type = @source_type)
  AND (@q::text = '' OR vj.query ILIKE '%' || @q || '%' OR vj.source_url ILIKE '%' || @q || '%' OR u.username ILIKE '%' || @q || '%');
