-- Revert region/subregion columns on spaces.

DROP INDEX IF EXISTS idx_spaces_region;
ALTER TABLE spaces DROP COLUMN IF EXISTS subregion;
ALTER TABLE spaces DROP COLUMN IF EXISTS region;
