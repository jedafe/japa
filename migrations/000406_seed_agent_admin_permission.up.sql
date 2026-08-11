-- 000406_seed_agent_admin_permission.up.sql
-- Seeds the agent.admin RBAC permission node used by the AI Agent admin
-- observability routes (GET /admin/agent/sessions, :id, tool-stats,
-- cost-report — Phase 6 task 6.08, wired in internal/server/server.go).
--
-- The node was referenced by server.go but never seeded in any migration,
-- so non-super_admin users with a custom 'admin' role could not access the
-- agent admin routes (403 forbidden). Super admins bypass all permission
-- checks, so the omission was masked in super-admin-only testing.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Admin Routes)
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 6 task 6.08)

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('agent.admin', 'AI Agent — view agent sessions, tool stats, and cost reports (admin observability)')
ON CONFLICT (key) DO NOTHING;

-- Grant to super_admin (also auto-granted by 000099 on fresh DBs, but
-- existing DBs need an explicit grant for the new node).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
CROSS JOIN permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'agent.admin'
ON CONFLICT DO NOTHING;

-- Grant to the 'admin' role if it exists (created manually via the admin
-- panel in some deployments). No-op if the role has not been created yet.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
CROSS JOIN permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'admin'
  AND p.key = 'agent.admin'
ON CONFLICT DO NOTHING;

COMMIT;
