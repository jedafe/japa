-- 000268 (down) — Revert page_type check to ('page', 'sidebar').
--
-- WARNING: any rows with page_type='landing' must be deleted or rewritten to
-- 'page' before applying this down migration, otherwise the ADD CONSTRAINT will
-- fail.

ALTER TABLE pages DROP CONSTRAINT IF EXISTS pages_page_type_check;
ALTER TABLE pages ADD CONSTRAINT pages_page_type_check
    CHECK (page_type IN ('page', 'sidebar'));
