-- 000302_seed_wiki_rbac_nodes.down.sql
-- Reverses 000302_seed_wiki_rbac_nodes.up.sql — removes the four wiki.*
-- permission nodes and all their role_permissions grants.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('wiki.edit', 'wiki.review', 'wiki.manage', 'wiki.lock')
);

DELETE FROM permissions
WHERE key IN ('wiki.edit', 'wiki.review', 'wiki.manage', 'wiki.lock');

COMMIT;
