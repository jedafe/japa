-- Reverse BUG-006: remove tenant_id from user_blocks.
DROP INDEX IF EXISTS idx_user_blocks_tenant;
ALTER TABLE user_blocks DROP COLUMN IF EXISTS tenant_id;
