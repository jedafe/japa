-- 000330_seed_arrival_checklist_rbac.down.sql
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE key = 'arrival_checklist.manage');
DELETE FROM permissions WHERE key = 'arrival_checklist.manage';
COMMIT;
