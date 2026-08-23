-- 000539_doc_page_feedback.down.sql
ALTER TABLE doc_pages
DROP COLUMN IF EXISTS helpful_yes,
DROP COLUMN IF EXISTS helpful_no;
