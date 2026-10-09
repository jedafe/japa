-- 000818_appeals_foundation.down.sql
DELETE FROM ticket_types WHERE slug = 'appeal' AND is_system = true;

DROP INDEX IF EXISTS idx_moderation_appeals_pending_unique;

ALTER TABLE moderation_appeals DROP CONSTRAINT moderation_appeals_status_check;
UPDATE moderation_appeals SET status = 'approved' WHERE status = 'overturn';
UPDATE moderation_appeals SET status = 'rejected' WHERE status = 'upheld';
ALTER TABLE moderation_appeals
    ADD CONSTRAINT moderation_appeals_status_check
    CHECK (status = ANY (ARRAY['pending','approved','rejected']));

ALTER TABLE moderation_appeals
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS resolution_note,
    DROP COLUMN IF EXISTS reviewed_by,
    DROP COLUMN IF EXISTS ticket_id;
