-- 000686_intel_raw_doc_parent.down.sql
-- Reverts 000686: drops intel_raw_documents.parent_document_id.

BEGIN;

DROP INDEX IF EXISTS idx_intel_raw_documents_parent;

ALTER TABLE intel_raw_documents
  DROP COLUMN IF EXISTS parent_document_id;

COMMIT;
