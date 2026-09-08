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
