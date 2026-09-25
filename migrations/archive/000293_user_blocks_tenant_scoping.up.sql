-- BUG-006: Add tenant_id to user_blocks for JEOS multi-tenancy conformance (§9).
-- The user_blocks table was created in migration 000025 without a tenant_id
-- column. Per JEOS §9 (RBAC & Multi-tenancy), all queries must be
-- tenant-scoped. User IDs are globally unique UUIDs so cross-tenant blocks
-- are practically impossible, but the column is required for conformance.
ALTER TABLE user_blocks
    ADD COLUMN IF NOT EXISTS tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE;

-- Backfill: resolve tenant_id from the blocker's tenant membership.
-- Users belong to exactly one tenant (users.tenant_id), so we can copy it.
UPDATE user_blocks ub
SET tenant_id = u.tenant_id
FROM users u
WHERE ub.blocker_id = u.id AND ub.tenant_id IS NULL;

-- Make tenant_id NOT NULL after backfill.
ALTER TABLE user_blocks ALTER COLUMN tenant_id SET NOT NULL;

-- Add tenant-scoped index for efficient lookups.
CREATE INDEX IF NOT EXISTS idx_user_blocks_tenant ON user_blocks(tenant_id);
