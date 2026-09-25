-- 000616_seed_agent_skip_hitl_permission.down.sql
-- Removes the agent.skip_hitl permission seeded by the up migration.

BEGIN;

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug = 'super_admin_role'
)
AND permission_id IN (
    SELECT id FROM permissions
    WHERE key = 'agent.skip_hitl'
);

DELETE FROM permissions WHERE key = 'agent.skip_hitl';

COMMIT;
