-- 000332_seed_credential_checker_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
    'credential_checker.view','credential_checker.manage','credential_checker.note'
));
DELETE FROM permissions WHERE key IN (
    'credential_checker.view','credential_checker.manage','credential_checker.note'
);
COMMIT;
