-- 000831_doc_page_categories.up.sql
-- §44 (2026-10-10 amendment): optional two-level taxonomy inside each docs
-- audience. NULL category = ungrouped page; NULL subcategory = top of the
-- category group. Presentation metadata only — no effect on gating/slugs.
BEGIN;

ALTER TABLE doc_pages
    ADD COLUMN IF NOT EXISTS category text;

ALTER TABLE doc_pages
    ADD COLUMN IF NOT EXISTS subcategory text;

CREATE INDEX IF NOT EXISTS idx_doc_pages_audience_category
    ON doc_pages (audience, category);

COMMIT;
