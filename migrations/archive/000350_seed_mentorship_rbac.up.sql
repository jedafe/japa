-- 000350_seed_mentorship_rbac.up.sql
-- RBAC nodes for Mentorship Matching (§102, ADR-110).
--   mentorship.become_mentor — elevated roles (professional, agent, moderator, admin, super_admin)
--   mentorship.request       — member+
--   mentorship.respond       — elevated roles (professional, agent, moderator, admin, super_admin)
--   mentorship.manage        — admin, super_admin
--
-- Adaptation: only super_admin, member, guest roles exist in the seed data.
-- Elevated-role permissions grant to super_admin; member+ grants to member
-- and super_admin. When professional/agent/moderator/admin roles are added
-- in the future, this seed should be extended.
--
-- Ref: japaconnect-engineering/sections/102-mentorship-matching.md
--      japaconnect-engineering/sections/03-rbac.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('mentorship.become_mentor', 'Opt in as a mentor (elevated roles)'),
    ('mentorship.request',       'Send a mentorship request (member+)'),
    ('mentorship.respond',       'Accept or decline mentorship requests (elevated roles)'),
    ('mentorship.manage',        'Admin-manage mentorship connections')
ON CONFLICT (key) DO NOTHING;

-- member+ grants
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'mentorship.request'
ON CONFLICT DO NOTHING;

-- elevated-role grants (super_admin stands in for professional/agent/moderator/admin)
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('mentorship.become_mentor', 'mentorship.respond', 'mentorship.manage')
ON CONFLICT DO NOTHING;

COMMIT;
