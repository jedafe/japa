-- checklist.sql — Visa Document Checklist queries (Section 70, ADR-070).
-- Covers per-user progress CRUD, selector data from published visa guides,
-- and admin progress/completion-stats queries.
-- Ref: japaconnect-engineering/sections/70-visa-checklist.md
--      japaconnect-engineering/adr/ADR-070-visa-document-checklist-generator.md

-- =====================================================================
-- Upsert a single checklist progress item (mark complete/incomplete + note).
-- The ON CONFLICT clause handles both insert and update in one statement.
-- =====================================================================

-- name: UpsertChecklistProgress :one
-- completed is NOT NULL (always provided by the service layer, which
-- resolves a nil completed from the PATCH body to the existing value before
-- calling). completed_at and notes are nullable so a toggle-only update
-- preserves the existing notes (COALESCE) and a note-only update preserves
-- the existing completed_at.
INSERT INTO checklist_progress (
    tenant_id, user_id, guide_id, item_key, completed, completed_at, notes
)
VALUES (@tenant_id, @user_id, @guide_id, @item_key, @completed, sqlc.narg(completed_at), sqlc.narg(notes))
ON CONFLICT (tenant_id, user_id, guide_id, item_key)
DO UPDATE SET
    completed    = EXCLUDED.completed,
    completed_at = COALESCE(EXCLUDED.completed_at, checklist_progress.completed_at),
    notes        = COALESCE(EXCLUDED.notes, checklist_progress.notes),
    updated_at   = now()
RETURNING *;

-- =====================================================================
-- Get all progress items for a user + guide.
-- =====================================================================

-- name: ListChecklistProgressByUserGuide :many
SELECT * FROM checklist_progress
WHERE tenant_id = @tenant_id AND user_id = @user_id AND guide_id = @guide_id
ORDER BY item_key;

-- =====================================================================
-- Reset all progress for a user + guide (delete all items).
-- =====================================================================

-- name: DeleteChecklistProgressByUserGuide :exec
DELETE FROM checklist_progress
WHERE tenant_id = @tenant_id AND user_id = @user_id AND guide_id = @guide_id;

-- =====================================================================
-- Count completed items for a user + guide (for completion check).
-- =====================================================================

-- name: CountCompletedChecklistItems :one
SELECT COUNT(*) AS count
FROM checklist_progress
WHERE tenant_id = @tenant_id AND user_id = @user_id AND guide_id = @guide_id
  AND completed = true;

-- =====================================================================
-- Admin: list all users' progress for a guide (paginated).
-- =====================================================================

-- name: AdminListChecklistProgress :many
SELECT
    cp.*,
    u.username AS username
FROM checklist_progress cp
JOIN users u ON u.id = cp.user_id
WHERE cp.tenant_id = @tenant_id
  AND (@guide_id::uuid IS NULL OR cp.guide_id = @guide_id)
  AND (@user_id::uuid IS NULL OR cp.user_id = @user_id)
ORDER BY cp.updated_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountChecklistProgress :one
SELECT COUNT(*) AS count
FROM checklist_progress
WHERE tenant_id = @tenant_id
  AND (@guide_id::uuid IS NULL OR guide_id = @guide_id)
  AND (@user_id::uuid IS NULL OR user_id = @user_id);

-- =====================================================================
-- Admin: completion stats by guide — for each guide with progress data,
-- returns total users, completed users (all items checked), and avg
-- completion percentage.
-- =====================================================================

-- name: AdminChecklistCompletionStats :many
SELECT
    cp.guide_id,
    COUNT(DISTINCT cp.user_id) AS total_users,
    COUNT(DISTINCT CASE WHEN cp.completed = true THEN cp.user_id END) AS users_with_completions
FROM checklist_progress cp
WHERE cp.tenant_id = @tenant_id
GROUP BY cp.guide_id
ORDER BY total_users DESC
LIMIT @page_limit OFFSET @page_offset;
