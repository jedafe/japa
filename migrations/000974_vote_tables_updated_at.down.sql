BEGIN;

DROP INDEX IF EXISTS idx_activity_events_undoes_pending;
ALTER TABLE comment_votes DROP COLUMN IF EXISTS updated_at;
ALTER TABLE forum_votes DROP COLUMN IF EXISTS updated_at;

COMMIT;
