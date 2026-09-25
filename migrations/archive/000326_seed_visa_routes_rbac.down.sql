-- 000326_seed_visa_routes_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN ('visa_routes.view','visa_routes.manage'));
DELETE FROM permissions WHERE key IN ('visa_routes.view','visa_routes.manage');
COMMIT;
