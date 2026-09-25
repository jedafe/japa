-- 000172_bookmarks_folder_id.up.sql
-- AUDIT-BE2-BL-011: bookmarks cannot be assigned to first-class bookmark
-- folders. Adds a nullable folder_id FK to bookmarks referencing
-- bookmark_folders(id). The existing free-text `folder` column (000103) is
-- left untouched for backward compatibility; folder_id is the preferred
-- association going forward. NULL means "no folder / unfiled".
--
-- Zero-downtime: additive, nullable, no backfill needed (existing rows keep
-- folder_id = NULL). ON DELETE SET NULL so deleting a folder unfiles its
-- bookmarks rather than cascading the delete.

BEGIN;

ALTER TABLE bookmarks
    ADD COLUMN IF NOT EXISTS folder_id uuid REFERENCES bookmark_folders(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_bookmarks_folder ON bookmarks(tenant_id, user_id, folder_id);

COMMIT;
