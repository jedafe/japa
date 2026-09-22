-- 000714_seed_sop_generator_rbac_nodes.down.sql
-- Reverses 000714: removes sop_generator.* grants and permission nodes.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('sop_generator.view', 'sop_generator.generate', 'sop_generator.manage')
);

DELETE FROM permissions
WHERE key IN ('sop_generator.view', 'sop_generator.generate', 'sop_generator.manage');

COMMIT;
