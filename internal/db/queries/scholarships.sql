-- scholarships.sql — Scholarship Finder queries (Section 73, ADR-073).
-- Covers saved scholarship tracker CRUD, finder queries on published
-- scholarship guides, and admin stats/deadline reports.
-- Ref: japaconnect-engineering/sections/73-scholarship-finder.md
--      japaconnect-engineering/adr/ADR-073-scholarship-finder.md

-- =====================================================================
-- Save a scholarship (insert into tracker).
-- =====================================================================

-- name: SaveScholarship :one
INSERT INTO saved_scholarships (user_id, guide_id, status, remind_before_days) VALUES (@user_id, @guide_id, 'saved', @remind_before_days)

ON CONFLICT (user_id, guide_id) DO NOTHING
RETURNING *;

-- =====================================================================
-- Update a saved scholarship's status/notes/reminder_days.
-- =====================================================================

-- name: UpdateSavedScholarship :one
UPDATE saved_scholarships
SET
 status = COALESCE(sqlc.narg(status), status),
 notes = COALESCE(sqlc.narg(notes), notes),
 remind_before_days = COALESCE(sqlc.narg(remind_before_days), remind_before_days),
 updated_at = now()
WHERE user_id = @user_id AND guide_id = @guide_id
RETURNING *;

-- =====================================================================
-- Unsave (delete) a scholarship from the tracker.
-- =====================================================================

-- name: DeleteSavedScholarship :exec
DELETE FROM saved_scholarships
WHERE user_id = @user_id AND guide_id = @guide_id;

-- =====================================================================
-- Get a single saved scholarship record.
-- =====================================================================

-- name: GetSavedScholarship :one
SELECT * FROM saved_scholarships
WHERE user_id = @user_id AND guide_id = @guide_id;

-- =====================================================================
-- List a user's saved scholarships with guide titles.
-- =====================================================================

-- name: ListSavedScholarships :many
SELECT
 ss.*,
 c.title AS guide_title,
 c.slug AS guide_slug
FROM saved_scholarships ss
JOIN contents c ON c.id = ss.guide_id
WHERE ss.user_id = @user_id
 AND (@status::text = '' OR ss.status = @status)
 AND (@guide_id::uuid IS NULL OR ss.guide_id = @guide_id)
ORDER BY ss.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountSavedScholarships :one
SELECT COUNT(*) AS count
FROM saved_scholarships
WHERE user_id = @user_id
 AND (@status::text = '' OR status = @status)
 AND (@guide_id::uuid IS NULL OR guide_id = @guide_id);

-- =====================================================================
-- Admin: save counts + status distribution.
-- =====================================================================

-- name: AdminScholarshipStatusDistribution :many
SELECT
 status,
 COUNT(*) AS count
FROM saved_scholarships

GROUP BY status
ORDER BY count DESC;

-- name: AdminScholarshipTopSaved :many
SELECT
 ss.guide_id,
 c.title AS guide_title,
 COUNT(*) AS save_count
FROM saved_scholarships ss
JOIN contents c ON c.id = ss.guide_id

GROUP BY ss.guide_id, c.title
ORDER BY save_count DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- Admin: upcoming deadline report — saved scholarships with deadlines
-- approaching within 30 days, joined with custom_field_values for the
-- application_deadline field.
-- =====================================================================

-- name: AdminScholarshipUpcomingDeadlines :many
SELECT
 ss.id,
 ss.user_id,
 ss.guide_id,
 ss.status,
 ss.remind_before_days,
 ss.last_reminded_at,
 c.title AS guide_title,
 u.username AS username,
 cfv.value_text AS deadline_value
FROM saved_scholarships ss
JOIN contents c ON c.id = ss.guide_id
JOIN users u ON u.id = ss.user_id
JOIN custom_field_values cfv ON cfv.entity_id = ss.guide_id
 AND cfv.entity_type = 'content'
JOIN custom_fields cf ON cf.id = cfv.field_id
 AND cf.key = 'application_deadline'
WHERE cfv.value_text IS NOT NULL
ORDER BY cfv.value_text ASC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- Deadline check task: find saved scholarships needing reminders.
-- Returns records where the deadline is within remind_before_days and
-- no reminder has been sent yet (or was sent more than 1 day ago).
--
-- The deadline is read from the structured `scholarships` table
-- (migration 429, ADR-112) via guide_id, replacing the previous
-- custom_field_values lookup. The `deadline` date column is cast to
-- text (YYYY-MM-DD) so the existing parseDeadlineValue/withinReminderWindow
-- logic in service.go continues to work unchanged.
-- =====================================================================

-- name: FindScholarshipsNeedingReminders :many
SELECT
 ss.id,
 ss.user_id,
 ss.guide_id,
 ss.remind_before_days,
 ss.last_reminded_at,
 s.deadline::text AS deadline_value,
 c.title AS guide_title,
 c.slug AS guide_slug
FROM saved_scholarships ss
JOIN contents c ON c.id = ss.guide_id
JOIN scholarships s ON s.guide_id = ss.guide_id
WHERE s.deadline IS NOT NULL
 AND (ss.last_reminded_at IS NULL OR ss.last_reminded_at < now() - interval '1 day')
LIMIT @batch_limit;

-- =====================================================================
-- Mark a reminder as sent (update last_reminded_at).
-- =====================================================================

-- name: MarkScholarshipReminderSent :exec
UPDATE saved_scholarships
SET last_reminded_at = now(),
 updated_at = now()
WHERE id = @id;

-- =====================================================================
-- Admin: reminder delivery history.
-- =====================================================================

-- name: AdminScholarshipReminderLog :many
SELECT
 ss.id,
 ss.user_id,
 ss.guide_id,
 ss.last_reminded_at,
 ss.status,
 c.title AS guide_title,
 u.username AS username
FROM saved_scholarships ss
JOIN contents c ON c.id = ss.guide_id
JOIN users u ON u.id = ss.user_id
WHERE ss.last_reminded_at IS NOT NULL
ORDER BY ss.last_reminded_at DESC
LIMIT @page_limit OFFSET @page_offset;
