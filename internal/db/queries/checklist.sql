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
-- list_type is hardcoded to 'visa' — this query is the §70 visa checklist
-- upsert. The §92 arrival checklist has its own store-level upsert. The
-- ON CONFLICT target includes list_type to match the 5-column unique
-- constraint added in migration 000381.
INSERT INTO checklist_progress (user_id, guide_id, item_key, completed, completed_at, notes, list_type) VALUES (@user_id, @guide_id, @item_key, @completed, sqlc.narg(completed_at), sqlc.narg(notes), 'visa')

ON CONFLICT (user_id, guide_id, item_key, list_type)
DO UPDATE SET
 completed = EXCLUDED.completed,
 completed_at = COALESCE(EXCLUDED.completed_at, checklist_progress.completed_at),
 notes = COALESCE(EXCLUDED.notes, checklist_progress.notes),
 updated_at = now()
RETURNING *;

-- =====================================================================
-- Get all progress items for a user + guide.
-- =====================================================================

-- name: ListChecklistProgressByUserGuide :many
SELECT id, user_id, guide_id, item_key, completed, completed_at, notes, created_at, updated_at, list_type FROM checklist_progress
WHERE user_id = @user_id AND guide_id = @guide_id
 AND list_type = 'visa'
ORDER BY item_key;

-- =====================================================================
-- Get a single progress item for a user + guide + item_key. Used by the
-- service to resolve the existing completed state for a note-only PATCH.
-- =====================================================================

-- name: GetSingleChecklistProgress :one
SELECT id, user_id, guide_id, item_key, completed, completed_at, notes, created_at, updated_at, list_type FROM checklist_progress
WHERE user_id = @user_id AND guide_id = @guide_id
 AND item_key = @item_key AND list_type = 'visa'
LIMIT 1;

-- =====================================================================
-- Reset all progress for a user + guide (delete all items).
-- =====================================================================

-- name: DeleteChecklistProgressByUserGuide :exec
DELETE FROM checklist_progress
WHERE user_id = @user_id AND guide_id = @guide_id
 AND list_type = 'visa';

-- =====================================================================
-- Count completed items for a user + guide (for completion check).
-- =====================================================================

-- name: CountCompletedChecklistItems :one
SELECT COUNT(*) AS count
FROM checklist_progress
WHERE user_id = @user_id AND guide_id = @guide_id
 AND completed = true AND list_type = 'visa';

-- =====================================================================
-- Admin: list all users' progress for a guide (paginated).
-- =====================================================================

-- name: AdminListChecklistProgress :many
SELECT
 cp.id, cp.user_id, cp.guide_id, cp.item_key, cp.completed, cp.completed_at, cp.notes, cp.created_at, cp.updated_at, cp.list_type,
 u.username AS username
FROM checklist_progress cp
JOIN users u ON u.id = cp.user_id
WHERE cp.list_type = 'visa'
 AND (@guide_id::uuid IS NULL OR cp.guide_id = @guide_id)
 AND (@user_id::uuid IS NULL OR cp.user_id = @user_id)
ORDER BY cp.updated_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: AdminCountChecklistProgress :one
SELECT COUNT(*) AS count
FROM checklist_progress
WHERE list_type = 'visa'
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
 COUNT(DISTINCT CASE WHEN cp.completed = true THEN cp.user_id END) AS users_with_any_completion
FROM checklist_progress cp
WHERE cp.list_type = 'visa'
GROUP BY cp.guide_id
ORDER BY total_users DESC
LIMIT @page_limit OFFSET @page_offset;
