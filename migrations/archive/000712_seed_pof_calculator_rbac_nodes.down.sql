-- 000712_seed_pof_calculator_rbac_nodes.down.sql
-- Reverses 000712: removes pof_calculator.* grants and permission nodes.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('pof_calculator.view', 'pof_calculator.manage')
);

DELETE FROM permissions
WHERE key IN ('pof_calculator.view', 'pof_calculator.manage');

COMMIT;
