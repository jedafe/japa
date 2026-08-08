-- 000334_seed_cv_formatter_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key IN (
    'cv_formatter.use','cv_formatter.unlimited','cv_formatter.manage'
));
DELETE FROM permissions WHERE key IN (
    'cv_formatter.use','cv_formatter.unlimited','cv_formatter.manage'
);
COMMIT;
