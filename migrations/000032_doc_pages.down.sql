-- Down migration for 000032_doc_pages.sql
-- Drops the doc_pages table (self-referencing FK on parent_id is handled by the drop).
DROP TABLE IF EXISTS doc_pages;
