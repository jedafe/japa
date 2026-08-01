-- WebSocket / DM schema reconciliation (BUG-001, BUG-002, BUG-007, BUG-008).
-- Ref: japaconnect-engineering/sections/48-direct-messages.md
--
-- 1. Migrate dm_message_requests → dm_requests and drop the old table.
--    The old dm_message_requests table (000015) only allowed
--    pending/accepted/declined in its CHECK constraint; dm_requests (000155)
--    is the spec-compliant table with tenant_id, requester_id, recipient_id,
--    preview_message, resolved_at, and a CHECK that includes 'blocked'.
-- 2. Add tenant_id to dm_messages (backfilled from dm_conversations.tenant_id).
-- 3. Add tenant_id to user_dm_settings (backfilled from users.tenant_id).

-- 1a. Backfill dm_requests from dm_message_requests + dm_conversations.
--     The old table only had conversation_id; we derive requester/recipient
--     from the conversation's participants. The requester is participant_1
--     (the lower UUID, per the CHECK constraint in dm_conversations) and the
--     recipient is participant_2. preview_message is set to '[migrated]'.
INSERT INTO dm_requests (tenant_id, requester_id, recipient_id, conversation_id, preview_message, status, created_at, updated_at, resolved_at)
SELECT
    dc.tenant_id,
    dc.participant_1_id,
    dc.participant_2_id,
    dmr.conversation_id,
    '[migrated]',
    dmr.status,
    dmr.created_at,
    dmr.created_at,
    CASE WHEN dmr.status IN ('accepted', 'declined', 'blocked') THEN dmr.created_at ELSE NULL END
FROM dm_message_requests dmr
JOIN dm_conversations dc ON dc.id = dmr.conversation_id
WHERE NOT EXISTS (
    SELECT 1 FROM dm_requests dr WHERE dr.conversation_id = dmr.conversation_id
);

-- 1b. Drop the old dm_message_requests table.
DROP TABLE IF EXISTS dm_message_requests;

-- 2. Add tenant_id to dm_messages and backfill from dm_conversations.
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE;

UPDATE dm_messages dm
SET tenant_id = dc.tenant_id
FROM dm_conversations dc
WHERE dm.conversation_id = dc.id AND dm.tenant_id IS NULL;

ALTER TABLE dm_messages ALTER COLUMN tenant_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_dm_messages_tenant ON dm_messages (tenant_id);

-- 3. Add tenant_id to user_dm_settings and backfill from users.
ALTER TABLE user_dm_settings ADD COLUMN IF NOT EXISTS tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE;

UPDATE user_dm_settings s
SET tenant_id = u.tenant_id
FROM users u
WHERE s.user_id = u.id AND s.tenant_id IS NULL;

ALTER TABLE user_dm_settings ALTER COLUMN tenant_id SET NOT NULL;

-- Drop the old user_id-only PK and create a composite PK (tenant_id, user_id)
-- so cross-tenant user_id collisions cannot leak settings.
ALTER TABLE user_dm_settings DROP CONSTRAINT user_dm_settings_pkey;
ALTER TABLE user_dm_settings ADD PRIMARY KEY (tenant_id, user_id);

CREATE INDEX IF NOT EXISTS idx_user_dm_settings_tenant ON user_dm_settings (tenant_id);
