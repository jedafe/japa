-- 000713_seed_eca_guide_rbac_nodes.down.sql
-- Reverses 000713: removes eca_guide.* grants and permission nodes.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('eca_guide.view', 'eca_guide.manage', 'eca_guide.update')
);

DELETE FROM permissions
WHERE key IN ('eca_guide.view', 'eca_guide.manage', 'eca_guide.update');

COMMIT;
