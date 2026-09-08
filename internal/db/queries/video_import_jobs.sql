-- Video import jobs: bulk YouTube channel/playlist/keyword imports.

-- name: CreateVideoImportJob :one
INSERT INTO video_import_jobs (
    tenant_id, user_id, source_type, source_url, query, target_author_user_id,
    status, total_items, imported_count, failed_count, error_log
) VALUES (
    @tenant_id, @user_id, @source_type, @source_url, @query, @target_author_user_id,
    COALESCE(@status, 'pending'), @total_items, @imported_count, @failed_count,
    COALESCE(@error_log, '[]'::jsonb)
)
RETURNING
    id, tenant_id, user_id, source_type, source_url, query, target_author_user_id,
    status, total_items, imported_count, failed_count, error_log, created_at, completed_at;

-- name: GetVideoImportJobByID :one
SELECT
    id, tenant_id, user_id, source_type, source_url, query, target_author_user_id,
    status, total_items, imported_count, failed_count, error_log, created_at, completed_at
FROM video_import_jobs
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListVideoImportJobsByUser :many
SELECT
    id, tenant_id, user_id, source_type, source_url, query, target_author_user_id,
    status, total_items, imported_count, failed_count, error_log, created_at, completed_at
FROM video_import_jobs
WHERE tenant_id = @tenant_id AND user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;
