-- Reverse of 000831 up.
BEGIN;

DROP INDEX IF EXISTS idx_doc_pages_audience_category;

ALTER TABLE doc_pages
    DROP COLUMN IF EXISTS category;

ALTER TABLE doc_pages
    DROP COLUMN IF EXISTS subcategory;

COMMIT;
