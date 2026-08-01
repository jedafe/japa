-- 000218_university_submission_tracking.down.sql
BEGIN;

ALTER TABLE learning_tracks
    DROP COLUMN IF EXISTS university_submission_status,
    DROP COLUMN IF EXISTS university_submitted_at,
    DROP COLUMN IF EXISTS university_reviewed_at,
    DROP COLUMN IF EXISTS university_rejection_reason,
    DROP COLUMN IF EXISTS university_featured;

DROP INDEX IF EXISTS idx_learning_tracks_submission_status;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'university.payout.view_all')
  AND role_id IN (SELECT id FROM user_roles WHERE slug = 'super_admin_role');
DELETE FROM permissions WHERE key = 'university.payout.view_all';

COMMIT;
