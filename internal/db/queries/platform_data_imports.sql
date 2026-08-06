-- Platform Data Imports — Section 67
-- Admin-initiated imports of platform data from CSV/JSON files.

-- name: CreatePlatformDataImport :one
INSERT INTO platform_data_imports (
    tenant_id, name, import_type, format, file_url, mode, created_by
) VALUES (
    @tenant_id, @name, @import_type, @format, @file_url, @mode, @created_by
)
RETURNING *;

-- name: GetPlatformDataImport :one
SELECT * FROM platform_data_imports
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetPlatformDataImportByID :one
-- Used by the Asynq worker, which does not have tenant context.
SELECT * FROM platform_data_imports
WHERE id = @id;

-- name: ListPlatformDataImports :many
SELECT * FROM platform_data_imports
WHERE tenant_id = @tenant_id
  AND (@status = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountPlatformDataImports :one
SELECT count(*) FROM platform_data_imports
WHERE tenant_id = @tenant_id
  AND (@status = '' OR status = @status);

-- name: UpdatePlatformDataImportStatus :one
UPDATE platform_data_imports
SET status = @status,
    total_rows = @total_rows,
    imported_rows = @imported_rows,
    failed_rows = @failed_rows,
    error_log = @error_log,
    completed_at = @completed_at
WHERE id = @id
RETURNING *;

-- name: DeletePlatformDataImport :exec
DELETE FROM platform_data_imports
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListPendingPlatformDataImports :many
SELECT * FROM platform_data_imports
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT @limit_val;
