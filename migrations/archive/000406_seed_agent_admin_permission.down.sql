-- 000406_seed_agent_admin_permission.down.sql
-- Reverts the agent.admin permission node seed.
-- Removes role_permissions grants for agent.admin, then deletes the node.

BEGIN;

DELETE FROM role_permissions
WHERE permission_id = (
    SELECT id FROM permissions WHERE key = 'agent.admin'
);

DELETE FROM permissions WHERE key = 'agent.admin';

COMMIT;
