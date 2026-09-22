-- 000711_seed_crs_calculator_rbac_nodes.down.sql
-- Reverses 000711: removes crs_calculator.* grants and permission nodes.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('crs_calculator.view', 'crs_calculator.manage', 'crs_calculator.history')
);

DELETE FROM permissions
WHERE key IN ('crs_calculator.view', 'crs_calculator.manage', 'crs_calculator.history');

COMMIT;
