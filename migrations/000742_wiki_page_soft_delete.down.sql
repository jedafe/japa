-- 000742_wiki_page_soft_delete.down.sql
-- Reverse the wiki page soft delete: drop the wiki.delete grants + node and
-- the deleted_at column.
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'wiki.delete');

DELETE FROM permissions WHERE key = 'wiki.delete';

ALTER TABLE wiki_pages DROP COLUMN IF EXISTS deleted_at;

COMMIT;
