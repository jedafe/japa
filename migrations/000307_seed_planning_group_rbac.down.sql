-- 000307_seed_planning_group_rbac.down.sql
BEGIN;
DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN
    ('planning_group.create','planning_group.manage_own','planning_group.manage_any'));
DELETE FROM permissions WHERE key IN
    ('planning_group.create','planning_group.manage_own','planning_group.manage_any');
COMMIT;
