DROP INDEX IF EXISTS idx_contents_org;
ALTER TABLE contents DROP COLUMN IF EXISTS organization_id;
