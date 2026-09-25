-- 000172_bookmarks_folder_id.down.sql
-- Reverts 000172: drops the folder_id FK column and its index from bookmarks.
-- The free-text `folder` column (000103) is untouched.

BEGIN;

DROP INDEX IF EXISTS idx_bookmarks_folder;
ALTER TABLE bookmarks DROP COLUMN IF EXISTS folder_id;

COMMIT;
