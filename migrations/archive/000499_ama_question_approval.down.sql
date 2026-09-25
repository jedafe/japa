-- 000499_ama_question_approval.down.sql
-- Reverses 000499: removes ama_pending and ama_approved_at from forum_replies.

BEGIN;

DROP INDEX IF EXISTS idx_forum_replies_ama_pending;
ALTER TABLE forum_replies
    DROP COLUMN IF EXISTS ama_approved_at,
    DROP COLUMN IF EXISTS ama_pending;

COMMIT;
