-- 000516_content_rbac_own_any.down.sql
-- Revert 000516: remove the underscore-scoped content edit/delete nodes and
-- their role grants.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN (
        'content.edit_own',
        'content.edit_any',
        'content.delete_own',
        'content.delete_any'
    )
);

DELETE FROM permissions
WHERE key IN (
    'content.edit_own',
    'content.edit_any',
    'content.delete_own',
    'content.delete_any'
);

COMMIT;
