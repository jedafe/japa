-- 000208_user_privacy_settings_tenant_id.up.sql
-- Add tenant_id to user_privacy_settings for multi-tenant isolation.
-- Ref: japaconnect-engineering/sections/04-multi-tenancy.md (BUG-009)
-- The table previously keyed only on user_id (PK), which is globally
-- unique, but adding tenant_id lets queries enforce tenant scoping and
-- keeps the table consistent with every other scoped table.

-- 1. Add the column as nullable first so existing rows backfill.
ALTER TABLE user_privacy_settings ADD COLUMN IF NOT EXISTS tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE;

-- 2. Backfill from users (every privacy row has a corresponding user).
UPDATE user_privacy_settings ups
SET tenant_id = u.tenant_id
FROM users u
WHERE ups.user_id = u.id AND ups.tenant_id IS NULL;

-- 3. Make NOT NULL now that every row has a value.
ALTER TABLE user_privacy_settings ALTER COLUMN tenant_id SET NOT NULL;

-- 4. Composite PK + tenant index for scoped queries.
ALTER TABLE user_privacy_settings DROP CONSTRAINT IF EXISTS user_privacy_settings_pkey;
ALTER TABLE user_privacy_settings ADD PRIMARY KEY (tenant_id, user_id);

CREATE INDEX IF NOT EXISTS idx_user_privacy_settings_tenant ON user_privacy_settings(tenant_id, user_id);
