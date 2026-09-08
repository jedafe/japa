-- 000626_seed_calendar_edit_permissions.down.sql
BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN ('calendar.edit_own', 'calendar.edit_any')
);

DELETE FROM permissions
WHERE key IN ('calendar.edit_own', 'calendar.edit_any');

COMMIT;
