-- Remove sidebar pages and the page_type column.

DELETE FROM pages WHERE page_type = 'sidebar';

DROP INDEX IF EXISTS idx_pages_page_type;

ALTER TABLE pages DROP CONSTRAINT IF EXISTS pages_page_type_check;

ALTER TABLE pages DROP COLUMN IF EXISTS page_type;
