-- 000851_unified_japa_ai.up.sql
-- ADR-130: Unified Japa AI — merged chat + agent surface (Phase 1).
--
-- The §103 agent loop becomes the single chat engine behind POST /ai/chat.
-- Transcripts move from Honcho-grouped memory entries to relational
-- ai_conversations/ai_messages; ai_agent_sessions stays the per-turn audit
-- record, now linked back to the conversation it served.
--
-- 1. ai_conversations — user-facing thread container (soft-deleted, Rule 11).
-- 2. ai_messages — transcript rows; agent_session_id links an assistant
--    message to its audit session.
-- 3. ai_agent_sessions + conversation_id (FK), + message_id (no FK —
--    ai_messages already references ai_agent_sessions; a second FK would be
--    circular. Application code keeps message_id consistent).
-- 4. user_preferences + ai_prefs jsonb — per-user AI defaults
--    (default_mode, disabled_categories, disabled_tools, enabled_tools).
-- 5. feature.japa_ai_unified flag + ai.* settings keys.

BEGIN;

-- ---------------------------------------------------------------------
-- 1. ai_conversations
-- ---------------------------------------------------------------------
CREATE TABLE ai_conversations (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);

CREATE INDEX idx_ai_conversations_user
    ON ai_conversations (user_id, updated_at DESC)
    WHERE deleted_at IS NULL;

-- ---------------------------------------------------------------------
-- 2. ai_messages
-- ---------------------------------------------------------------------
CREATE TABLE ai_messages (
    id               uuid          PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id  uuid          NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role             text          NOT NULL,
    content          text          NOT NULL DEFAULT '',
    model            text,
    voice_id         text,
    tokens_used      integer       NOT NULL DEFAULT 0,
    cost_usd         numeric(10,4) NOT NULL DEFAULT 0,
    agent_session_id uuid          REFERENCES ai_agent_sessions(id) ON DELETE SET NULL,
    created_at       timestamptz   NOT NULL DEFAULT now(),

    CONSTRAINT ai_messages_role_chk CHECK (role IN ('user','assistant'))
);

CREATE INDEX idx_ai_messages_conversation
    ON ai_messages (conversation_id, created_at);

-- ---------------------------------------------------------------------
-- 3. ai_agent_sessions — conversation/audit links
-- ---------------------------------------------------------------------
ALTER TABLE ai_agent_sessions
    ADD COLUMN conversation_id uuid REFERENCES ai_conversations(id) ON DELETE SET NULL,
    ADD COLUMN message_id      uuid;

CREATE INDEX idx_ai_agent_sessions_conversation
    ON ai_agent_sessions (conversation_id)
    WHERE conversation_id IS NOT NULL;

-- ---------------------------------------------------------------------
-- 4. user_preferences.ai_prefs — per-user AI defaults
-- ---------------------------------------------------------------------
ALTER TABLE user_preferences
    ADD COLUMN ai_prefs jsonb NOT NULL DEFAULT '{}'::jsonb;

-- ---------------------------------------------------------------------
-- 5. Feature flag + settings keys
-- ---------------------------------------------------------------------
-- Unified Japa AI rollout flag — default off; enable per-environment via
-- Control → Extensions once the merged surface is verified.
INSERT INTO feature_flags (key, enabled, updated_at)
SELECT 'feature.japa_ai_unified', false, now()
ON CONFLICT (key) DO NOTHING;

INSERT INTO system_settings (key, value, is_public)
SELECT 'ai.history_window', '20', false
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE system_settings.key = 'ai.history_window');

INSERT INTO system_settings (key, value, is_public)
SELECT 'ai.conversation_title_length', '80', false
WHERE NOT EXISTS (SELECT 1 FROM system_settings WHERE system_settings.key = 'ai.conversation_title_length');

COMMIT;
