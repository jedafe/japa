-- 000851_unified_japa_ai.down.sql
-- Revert ADR-130 Phase 1 schema: drop unified chat tables, agent session
-- links, ai_prefs column, rollout flag, and settings keys.

BEGIN;

DROP TABLE IF EXISTS ai_messages;

-- Drop the session link columns BEFORE ai_conversations — the
-- ai_agent_sessions_conversation_id_fkey FK depends on the parent table.
ALTER TABLE ai_agent_sessions
    DROP COLUMN IF EXISTS conversation_id,
    DROP COLUMN IF EXISTS message_id;

DROP TABLE IF EXISTS ai_conversations;

ALTER TABLE user_preferences
    DROP COLUMN IF EXISTS ai_prefs;

DELETE FROM feature_flags WHERE key = 'feature.japa_ai_unified';
DELETE FROM system_settings WHERE key IN ('ai.history_window', 'ai.conversation_title_length');

COMMIT;
