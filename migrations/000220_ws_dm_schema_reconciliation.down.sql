-- Reverse of 000220_ws_dm_schema_reconciliation.up.sql

-- 3. Remove tenant_id from user_dm_settings.
DROP INDEX IF EXISTS idx_user_dm_settings_tenant;
ALTER TABLE user_dm_settings DROP CONSTRAINT IF EXISTS user_dm_settings_pkey;
ALTER TABLE user_dm_settings ADD PRIMARY KEY (user_id);
ALTER TABLE user_dm_settings DROP COLUMN IF EXISTS tenant_id;

-- 2. Remove tenant_id from dm_messages.
DROP INDEX IF EXISTS idx_dm_messages_tenant;
ALTER TABLE dm_messages DROP COLUMN IF EXISTS tenant_id;

-- 1. Recreate dm_message_requests (data was migrated to dm_requests).
CREATE TABLE IF NOT EXISTS dm_message_requests (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES dm_conversations(id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'pending',
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(conversation_id)
);
