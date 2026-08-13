-- 000452_seed_ai_provider_budget_rbac.up.sql
-- Adds granular RBAC permission nodes for the Control Panel AI provider and
-- budget routes. Previously these routes were only gated by control.panel.access,
-- meaning any user with control panel access could create/delete AI providers
-- (including API keys) and modify budget thresholds. The new nodes allow
-- delegating narrow access without granting full provider CRUD.
--
-- Nodes:
--   ai.providers.manage — Create, update, delete, test AI providers (super_admin)
--   ai providers.view    — View AI provider list and config (read-only)
--   ai budget.view       — View AI budget dashboard (read-only)
--   ai budget.manage     — Update AI budget threshold
--
-- Ref: japaconnect-engineering/sections/19-ai-gateway.md
--      japaconnect-engineering/sections/21-control-panel.md
--      BUG-007 in .jeos/audit/ai-audit.md

BEGIN;

-- 1. Seed the four new RBAC permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('ai.providers.manage', 'Create, update, delete, and test AI provider configurations (API keys, base URLs, models)'),
    ('ai.providers.view',   'View AI provider list and configuration (read-only)'),
    ('ai.budget.view',      'View AI budget dashboard and spend metrics (read-only)'),
    ('ai.budget.manage',    'Update the AI monthly budget threshold')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant all four nodes to super_admin (admin capability).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('ai.providers.manage', 'ai.providers.view', 'ai.budget.view', 'ai.budget.manage')
ON CONFLICT DO NOTHING;

COMMIT;
