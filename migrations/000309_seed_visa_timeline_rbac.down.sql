-- 000309_seed_visa_timeline_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN
    ('visa_timeline.create','visa_timeline.manage_own','visa_timeline.manage_any'));
DELETE FROM permissions WHERE key IN
    ('visa_timeline.create','visa_timeline.manage_own','visa_timeline.manage_any');
COMMIT;
