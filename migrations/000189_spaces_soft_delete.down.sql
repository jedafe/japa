-- 000189 down: remove soft-delete from spaces.
DROP INDEX IF EXISTS idx_spaces_active;
ALTER TABLE spaces DROP COLUMN IF EXISTS deleted_at;
