-- 000138_seed_new_entity_rbac_nodes.down.sql
-- Removes ADR-054 permission nodes.

BEGIN;

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('menus.manage', 'email.templates.manage', 'redirects.manage')
);
DELETE FROM permissions WHERE key IN ('menus.manage', 'email.templates.manage', 'redirects.manage');

COMMIT;
