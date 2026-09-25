-- 000616_seed_agent_skip_hitl_permission.up.sql
-- Seeds the agent.skip_hitl permission node and grants it to the
-- super_admin_role so super-admin-initiated agent actions (content.create,
-- agent draft requests, studio jobs) bypass the HITL review queue. Other
-- user groups/roles can be granted the node through the RBAC permission
-- matrix to selectively skip review for trusted actors. The SystemWildcard
-- permission ('*') also satisfies the check implicitly.

BEGIN;

-- 1. Ensure the permission node exists.
INSERT INTO permissions (key, description) VALUES
    ('agent.skip_hitl', 'Skip HITL review for agent-initiated content actions')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant it to the super_admin_role (idempotent).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key = 'agent.skip_hitl'
ON CONFLICT DO NOTHING;

COMMIT;
