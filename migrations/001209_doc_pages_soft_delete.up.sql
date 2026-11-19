-- Rule 11: soft delete for doc_pages — deleted_at marks a trashed page;
-- public reads, admin lists, nav, and search exclude it, restore clears it,
-- and only ?force=true admin calls on soft-deleted rows may hard-delete.
ALTER TABLE doc_pages ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_doc_pages_deleted_at
    ON doc_pages (deleted_at)
    WHERE deleted_at IS NOT NULL;
