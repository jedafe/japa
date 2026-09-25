-- 000336_seed_housing_guide_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
    'housing_guide.view','housing_guide.manage'
));
DELETE FROM permissions WHERE key IN (
    'housing_guide.view','housing_guide.manage'
);
COMMIT;
