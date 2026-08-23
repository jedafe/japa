-- 000539_doc_page_feedback.up.sql
-- Aggregate helpful/unhelpful vote counters for doc pages (DOC-011).

ALTER TABLE doc_pages
ADD COLUMN IF NOT EXISTS helpful_yes int NOT NULL DEFAULT 0,
ADD COLUMN IF NOT EXISTS helpful_no int NOT NULL DEFAULT 0;
