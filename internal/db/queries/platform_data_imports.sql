-- Platform Data Imports — Section 67
-- Admin-initiated imports of platform data from CSV/JSON files.

-- name: CreatePlatformDataImport :one
INSERT INTO platform_data_imports (name, import_type, format, file_url, mode, created_by) VALUES (@name, @import_type, @format, @file_url, @mode, @created_by)
RETURNING *;

-- name: GetPlatformDataImport :one
SELECT * FROM platform_data_imports
WHERE id = @id;

-- name: GetPlatformDataImportByID :one
-- Used by the Asynq worker, which does not have tenant context.
SELECT * FROM platform_data_imports
WHERE id = @id;

-- name: ListPlatformDataImports :many
SELECT * FROM platform_data_imports
WHERE (@status = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountPlatformDataImports :one
SELECT count(*) FROM platform_data_imports
WHERE (@status = '' OR status = @status);

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
WHERE id = @id;

-- name: ListPendingPlatformDataImports :many
SELECT * FROM platform_data_imports
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT @limit_val;

-- name: ReassignPlatformDataImportCreatedBy :exec
-- Reassign created_by to a placeholder user during GDPR erasure so the
-- import audit trail survives when the admin who created the import is
-- erased (BUG-013, Section 67 / Section 15 compliance).
UPDATE platform_data_imports
SET created_by = @target_user_id
WHERE created_by = @user_id;
