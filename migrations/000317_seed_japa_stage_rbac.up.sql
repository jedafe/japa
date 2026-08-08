-- 000317_seed_japa_stage_rbac.up.sql
-- RBAC nodes for Japa Stage Progress Profiles (§87, ADR-095).
--   profile.japa_stage.edit — self (enforced in handler)
--   profile.japa_stage.view — all (if is_public)

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('profile.japa_stage.edit', 'Edit own japa stage profile (self)'),
    ('profile.japa_stage.view', 'View public japa stage profiles (all)')
ON CONFLICT (key) DO NOTHING;

-- Grant view to all roles including guest.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.key = 'profile.japa_stage.view'
ON CONFLICT DO NOTHING;

-- Grant edit to all authenticated roles (self-check in handler).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'profile.japa_stage.edit'
ON CONFLICT DO NOTHING;

COMMIT;
