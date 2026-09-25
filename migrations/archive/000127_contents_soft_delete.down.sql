DROP INDEX IF EXISTS idx_contents_soft_delete;
ALTER TABLE contents DROP COLUMN IF EXISTS deleted_at;
