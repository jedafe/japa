-- §115 AI Quick Panel: per-page thread anchoring. One live conversation
-- per (user_id, anchor); soft-deleting a thread frees the anchor so the
-- panel starts fresh on next send.
ALTER TABLE ai_conversations ADD COLUMN anchor text;

CREATE UNIQUE INDEX idx_ai_conversations_anchor
    ON ai_conversations (user_id, anchor)
    WHERE anchor IS NOT NULL AND deleted_at IS NULL;
