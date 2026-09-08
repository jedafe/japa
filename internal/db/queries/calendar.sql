-- Community Calendar queries (§86, ADR-094).
-- Fixed-shape queries are sqlc-generated here. Dynamic-filter queries
-- (ListEntries, AdminListEntries, UpcomingEntriesForReminders) remain
-- hand-written in internal/calendar/store.go because sqlc cannot express
-- dynamic WHERE clauses or dynamic OR builders.
-- Ref: japaconnect-engineering/sections/86-community-calendar.md

-- name: CalendarCreateEntry :one
INSERT INTO calendar_entries (
    tenant_id, title, description, category,
    destination_space_id, origin_space_id,
    occurs_at, ends_at, is_recurring, recurrence_pattern,
    source_url, status, submitted_by, staleness_check_due
) VALUES (
    $1, $2, $3, $4,
    $5, $6,
    $7, $8, $9, $10,
    $11, 'pending', $12, $13
)
RETURNING id;

-- name: CalendarSetStalenessCheckDue :exec
UPDATE calendar_entries
SET staleness_check_due = CASE
    WHEN is_recurring THEN occurs_at - INTERVAL '60 days'
    ELSE NULL
END
WHERE id = $1;

-- name: CalendarFlagStaleEntries :execrows
UPDATE calendar_entries
SET status = 'unverified', updated_at = now()
WHERE staleness_check_due IS NOT NULL
  AND staleness_check_due <= now()
  AND status = 'published'
  AND deleted_at IS NULL;

-- name: CalendarGetEntry :one
SELECT id, tenant_id, title, description, category,
       destination_space_id, origin_space_id,
       occurs_at, ends_at, is_recurring, recurrence_pattern,
       source_url, status, submitted_by, reviewed_by, reviewed_at,
       staleness_check_due,
       ai_generated, ai_model, hitl_reviewed_by, ai_generated_at,
       ai_prompt_hash, hitl_reviewed_at, hitl_review_notes,
       created_at, updated_at, deleted_at
FROM calendar_entries
WHERE id = $1 AND tenant_id = $2 AND status = 'published'
  AND deleted_at IS NULL
LIMIT 1;

-- name: CalendarSubscribersForEntry :many
SELECT user_id FROM calendar_subscriptions
WHERE tenant_id = $1
  AND destination_space_id = $2
  AND (category IS NULL OR category = $3 OR category = '');

-- name: CalendarSaveEntry :exec
INSERT INTO calendar_saves (user_id, entry_id, tenant_id, saved_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (user_id, entry_id) DO NOTHING;

-- name: CalendarUnsaveEntry :exec
DELETE FROM calendar_saves
WHERE user_id = $1 AND entry_id = $2 AND tenant_id = $3;

-- name: CalendarListSavedEntries :many
SELECT e.id, e.tenant_id, e.title, e.description, e.category,
       e.destination_space_id, e.origin_space_id,
       e.occurs_at, e.ends_at, e.is_recurring, e.recurrence_pattern,
       e.source_url, e.status, e.submitted_by, e.reviewed_by, e.reviewed_at,
       e.staleness_check_due,
       e.ai_generated, e.ai_model, e.hitl_reviewed_by, e.ai_generated_at,
       e.ai_prompt_hash, e.hitl_reviewed_at, e.hitl_review_notes,
       e.created_at, e.updated_at, e.deleted_at,
       COUNT(*) OVER() AS total_count
FROM calendar_saves s
JOIN calendar_entries e ON e.id = s.entry_id
WHERE s.user_id = $1 AND s.tenant_id = $2
  AND e.status = 'published' AND e.deleted_at IS NULL
ORDER BY s.saved_at DESC
LIMIT $3 OFFSET $4;

-- name: CalendarSubscribe :exec
-- category is NULL for "all categories" and must remain SQL NULL, not an
-- empty string, so the unique constraint and matching work correctly.
INSERT INTO calendar_subscriptions (user_id, tenant_id, destination_space_id, category, created_at)
VALUES ($1, $2, $3, NULLIF($4, ''), now())
ON CONFLICT (tenant_id, user_id, destination_space_id, category) DO NOTHING;

-- name: CalendarUnsubscribe :exec
DELETE FROM calendar_subscriptions
WHERE user_id = $1 AND tenant_id = $2 AND destination_space_id = $3
  AND (category = $4 OR (category IS NULL AND $4 IS NULL));

-- name: CalendarIsSaved :one
-- Returns true if the given user has saved the given entry.
SELECT EXISTS (
    SELECT 1 FROM calendar_saves
    WHERE user_id = $1 AND entry_id = $2 AND tenant_id = $3
) AS exists;

-- name: CalendarGetEntryForReview :one
-- Fetches the fields needed for the ADR-088 HITL gate check before
-- publishing an AI-generated entry. Used by Service.AdminUpdate.
SELECT id, ai_generated, hitl_reviewed_by, hitl_review_notes, source_url, status
FROM calendar_entries
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
LIMIT 1;

-- name: CalendarAdminUpdateEntry :execrows
-- On publish, stamps hitl_reviewed_by / hitl_reviewed_at for AI-generated
-- entries (ADR-088 HITL gate). Non-AI entries leave hitl_* unchanged.
UPDATE calendar_entries
SET status = $3,
    reviewed_by = $4,
    reviewed_at = now(),
    updated_at = now(),
    hitl_reviewed_by = CASE
        WHEN $3 = 'published' AND ai_generated THEN $4
        ELSE hitl_reviewed_by
    END,
    hitl_reviewed_at = CASE
        WHEN $3 = 'published' AND ai_generated THEN now()
        ELSE hitl_reviewed_at
    END,
    hitl_review_notes = CASE
        WHEN $3 = 'published' AND ai_generated THEN COALESCE(NULLIF($5, ''), hitl_review_notes)
        WHEN $5 IS NOT NULL AND $5 <> '' THEN COALESCE(NULLIF($5, ''), hitl_review_notes)
        ELSE hitl_review_notes
    END
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL;

-- name: CalendarAdminSoftDeleteEntry :execrows
UPDATE calendar_entries
SET deleted_at = now(), updated_at = now()
WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL;
