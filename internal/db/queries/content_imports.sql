-- content_imports.sql — Content Import & Migration Tools queries (Section 65, ADR-083).
-- Covers import tracking CRUD and admin oversight.
-- Ref: japaconnect-engineering/sections/65-content-import.md
--      japaconnect-engineering/adr/ADR-083-content-import.md

-- =====================================================================
-- Create an import record (pending status).
-- =====================================================================

-- name: CreateContentImport :one
INSERT INTO content_imports (tenant_id, user_id, source_type, source_url)
VALUES (@tenant_id, @user_id, @source_type, sqlc.narg(source_url))
RETURNING *;

-- =====================================================================
-- Get a single import by ID.
-- =====================================================================

-- name: GetContentImportByID :one
SELECT * FROM content_imports
WHERE tenant_id = @tenant_id AND id = @id;

-- =====================================================================
-- List a user's imports.
-- =====================================================================

-- name: ListUserContentImports :many
SELECT * FROM content_imports
WHERE tenant_id = @tenant_id AND user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUserContentImports :one
SELECT COUNT(*) AS count
FROM content_imports
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- =====================================================================
-- Update import status + counts (called by the import:process_source task).
-- =====================================================================

-- name: UpdateContentImportStatus :one
UPDATE content_imports
SET
    status          = @status,
    total_items     = COALESCE(sqlc.narg(total_items), total_items),
    imported_count  = COALESCE(sqlc.narg(imported_count), imported_count),
    failed_count    = COALESCE(sqlc.narg(failed_count), failed_count),
    error_log       = COALESCE(sqlc.narg(error_log), error_log),
    completed_at    = COALESCE(sqlc.narg(completed_at), completed_at)
WHERE tenant_id = @tenant_id AND id = @id
RETURNING *;

-- =====================================================================
-- Admin: list all imports across all users.
-- =====================================================================

-- name: AdminListContentImports :many
SELECT
    ci.*,
    u.username AS username
FROM content_imports ci
JOIN users u ON u.id = ci.user_id
WHERE ci.tenant_id = @tenant_id
  AND (@status::text = '' OR ci.status = @status)
ORDER BY ci.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountContentImports :one
SELECT COUNT(*) AS count
FROM content_imports
WHERE tenant_id = @tenant_id
  AND (@status::text = '' OR status = @status);

-- =====================================================================
-- Admin: retry a failed import (reset to pending).
-- =====================================================================

-- name: ResetContentImportForRetry :one
UPDATE content_imports
SET
    status          = 'pending',
    total_items     = 0,
    imported_count  = 0,
    failed_count    = 0,
    error_log       = '[]'::jsonb,
    completed_at    = NULL
WHERE tenant_id = @tenant_id AND id = @id AND status IN ('failed', 'partial')
RETURNING *;
