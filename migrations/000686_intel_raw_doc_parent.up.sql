-- 000686_intel_raw_doc_parent.up.sql
-- intel-channels Phase 2: raw docs produced by manifest fan-out (sitemap /
-- wayback child fetches) trace back to the manifest document that spawned
-- them. detect_changes uses the column to distinguish manifest bodies
-- (chain ends after change detection) from derived page bodies (normal
-- pipeline dispatch).
-- Ref: docs/superpowers/specs/2026-09-20-intel-channels-curation-design.md §3

BEGIN;

ALTER TABLE intel_raw_documents
  ADD COLUMN IF NOT EXISTS parent_document_id uuid
    REFERENCES intel_raw_documents(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_intel_raw_documents_parent
  ON intel_raw_documents(parent_document_id) WHERE parent_document_id IS NOT NULL;

COMMIT;
