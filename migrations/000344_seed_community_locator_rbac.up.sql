-- 000344_seed_community_locator_rbac.up.sql
-- RBAC nodes for Diaspora Community Locator (§101, ADR-109).
--   community_locator.submit  — member+
--   community_locator.review  — moderator, admin, super_admin
--   community_locator.claim   — org_member, professional, agent
--   community_locator.manage  — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('community_locator.submit', 'Submit a community organisation listing (member+)'),
    ('community_locator.review', 'Review and publish community submissions (moderator+)'),
    ('community_locator.claim',  'Initiate claim flow for a community organisation'),
    ('community_locator.manage', 'Manage community locator data (admin+)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key IN ('community_locator.submit','community_locator.claim')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key IN ('community_locator.review','community_locator.manage')
ON CONFLICT DO NOTHING;

COMMIT;
