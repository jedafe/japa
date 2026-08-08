DROP INDEX IF EXISTS idx_forum_tags_tenant_active;
ALTER TABLE forum_tags DROP COLUMN IF EXISTS deleted_at;
