-- 000852_agent_session_conversation_lock — One running agent turn per
-- unified conversation (ADR-130).
--
-- Without this, two concurrent sends against the same ai_conversations
-- thread interleave their messages (user, user, assistant, assistant)
-- because Resolve + session creation are not atomic. A partial unique
-- index makes "one running turn per conversation" a database invariant —
-- the second insert fails with a unique violation the app maps to
-- 409 conversation_busy.
--
-- Ref: japaconnect-engineering/adr/ADR-130-unified-japa-ai.md

BEGIN;

CREATE UNIQUE INDEX IF NOT EXISTS ai_agent_sessions_one_running_per_conv
    ON ai_agent_sessions (conversation_id)
    WHERE status = 'running' AND conversation_id IS NOT NULL;

COMMIT;
