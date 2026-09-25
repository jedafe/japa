-- 000731_wiki_community_fields.down.sql
-- Reverses 000731: removes the wiki.fields.* grants and permission nodes,
-- drops the community-fields columns from wiki_revisions, wiki_pages, and
-- comparison_dimensions, and restores wiki_revisions.wiki_page_id NOT NULL.
-- Field-only revisions (wiki_page_id IS NULL) cannot satisfy the restored
-- constraint, so they are deleted first.

BEGIN;

-- 1. Remove RBAC grants, then the permission nodes (mirrors 000730 down).
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('wiki.fields.edit', 'wiki.fields.propose', 'wiki.fields.manage')
);

DELETE FROM permissions
WHERE key IN ('wiki.fields.edit', 'wiki.fields.propose', 'wiki.fields.manage');

-- 2. Drop field-only revisions, then restore wiki_page_id NOT NULL and drop
-- the new columns (indexes/constraint on dropped columns are removed with them).
DELETE FROM wiki_revisions WHERE wiki_page_id IS NULL;

ALTER TABLE wiki_revisions
    ALTER COLUMN wiki_page_id SET NOT NULL,
    DROP COLUMN target_entity_id,
    DROP COLUMN target_entity_type,
    DROP COLUMN ops,
    DROP COLUMN kind;

-- 3. comparison_dimensions + wiki_pages column drops.
ALTER TABLE comparison_dimensions DROP COLUMN source_field_id;
ALTER TABLE wiki_pages DROP COLUMN data_entity_type;

COMMIT;
