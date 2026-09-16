-- Platform Data Exports — Section 67
-- Admin-initiated exports of platform data (users, posts, comments, etc.)

-- name: CreatePlatformDataExport :one
INSERT INTO platform_data_exports (name, export_type, format, include_metadata, created_by) VALUES (@name, @export_type, @format, @include_metadata, @created_by)
RETURNING *;

-- name: GetPlatformDataExport :one
SELECT * FROM platform_data_exports
WHERE id = @id;

-- name: GetPlatformDataExportByID :one
-- Used by the Asynq worker, which does not have platform context.
SELECT * FROM platform_data_exports
WHERE id = @id;

-- name: ListPlatformDataExports :many
SELECT * FROM platform_data_exports
WHERE (@status = '' OR status = @status)
ORDER BY created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: CountPlatformDataExports :one
SELECT count(*) FROM platform_data_exports
WHERE (@status = '' OR status = @status);

-- name: UpdatePlatformDataExportStatus :one
UPDATE platform_data_exports
SET status = @status,
 file_url = @file_url,
 file_size = @file_size,
 error_message = @error_message,
 completed_at = @completed_at
WHERE id = @id
RETURNING *;

-- name: DeletePlatformDataExport :exec
DELETE FROM platform_data_exports
WHERE id = @id;

-- name: ListPendingPlatformDataExports :many
SELECT * FROM platform_data_exports
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT @limit_val;

-- name: ReassignPlatformDataExportCreatedBy :exec
-- Reassign created_by to a placeholder user during GDPR erasure so the
-- export audit trail survives when the admin who created the export is
-- erased (BUG-010, Section 67 / Section 15 compliance).
UPDATE platform_data_exports
SET created_by = @target_user_id
WHERE created_by = @user_id;
