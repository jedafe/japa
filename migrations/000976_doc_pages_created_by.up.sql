-- 000976_doc_pages_created_by
-- Track the doc page author so the §44 review workflow can enforce
-- author ≠ reviewer on ai_generated pages (HITL). Nullable — legacy rows
-- and worker-generated drafts (no request user) stay NULL and remain
-- approvable by any docs.manage reviewer.
BEGIN;

ALTER TABLE doc_pages ADD COLUMN IF NOT EXISTS created_by uuid REFERENCES users (id);

COMMIT;
