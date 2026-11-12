-- 001122 down — remove email.providers.* nodes and their role grants.
BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('email.providers.manage', 'email.providers.view')
);

DELETE FROM permissions
WHERE key IN ('email.providers.manage', 'email.providers.view');

COMMIT;
