DROP INDEX IF EXISTS idx_doc_pages_deleted_at;
ALTER TABLE doc_pages DROP COLUMN IF EXISTS deleted_at;
