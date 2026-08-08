-- 000315_seed_calendar_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN
    ('calendar.submit','calendar.review','calendar.manage'));
DELETE FROM permissions WHERE key IN
    ('calendar.submit','calendar.review','calendar.manage');
COMMIT;
