-- 000609_seed_tier_roles.up.sql
-- Seed pro_member and business_member roles for the unified tier system,
-- copy the base member permissions, and assign the roles to the pro/business
-- tier groups. Ref: docs/superpowers/specs/2026-09-03-unified-user-tiers-and-professional-profile-removal-design.md

BEGIN;

-- Create the tier roles for the default tenant if they do not exist.
INSERT INTO user_roles (id, tenant_id, name, slug)
VALUES
    ('00000000-0000-0000-0000-000000000201', '00000000-0000-0000-0000-000000000001', 'Pro Member', 'pro_member'),
    ('00000000-0000-0000-0000-000000000202', '00000000-0000-0000-0000-000000000001', 'Business Member', 'business_member')
ON CONFLICT (tenant_id, slug) DO UPDATE SET
    name = EXCLUDED.name;

-- Grant the new roles the same permissions as the base `member` role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT pro.id, rp.permission_id
FROM user_roles pro
JOIN user_roles member ON member.tenant_id = '00000000-0000-0000-0000-000000000001' AND member.slug = 'member'
JOIN role_permissions rp ON rp.role_id = member.id
WHERE pro.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND pro.slug = 'pro_member'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT biz.id, rp.permission_id
FROM user_roles biz
JOIN user_roles member ON member.tenant_id = '00000000-0000-0000-0000-000000000001' AND member.slug = 'member'
JOIN role_permissions rp ON rp.role_id = member.id
WHERE biz.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND biz.slug = 'business_member'
ON CONFLICT DO NOTHING;

-- Assign the roles to the pro and business tier groups.
INSERT INTO group_roles (group_id, role_id)
SELECT g.id, r.id
FROM user_groups g
JOIN user_roles r ON r.tenant_id = g.tenant_id AND r.slug = 'pro_member'
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND g.slug = 'pro'
ON CONFLICT DO NOTHING;

INSERT INTO group_roles (group_id, role_id)
SELECT g.id, r.id
FROM user_groups g
JOIN user_roles r ON r.tenant_id = g.tenant_id AND r.slug = 'business_member'
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND g.slug = 'business'
ON CONFLICT DO NOTHING;

COMMIT;
