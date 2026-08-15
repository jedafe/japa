-- 000499_ama_question_approval.up.sql
-- BUG-003 (ama-audit): question_approval_required was stored in ama_config
-- but never enforced. This migration adds the columns needed to hold AMA
-- questions (forum_replies) for host approval before they become visible.
-- Ref: japaconnect-engineering/sections/88-ask-an-expat.md

BEGIN;

ALTER TABLE forum_replies
    ADD COLUMN IF NOT EXISTS ama_pending     boolean      NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS ama_approved_at timestamptz;

-- Partial index: only pending AMA questions need scanning.
CREATE INDEX IF NOT EXISTS idx_forum_replies_ama_pending
    ON forum_replies(thread_id, tenant_id)
    WHERE ama_pending = true AND deleted_at IS NULL;

COMMIT;
