-- 000618_grant_follows_toggle.up.sql
-- Grants the follows.toggle node to the member-tier roles. The node was
-- seeded by migration 000109 but never granted, while the seed granted
-- follows.create instead — meanwhile every follow route (spaces, tags,
-- organizations, users, scholarships, professional profiles) enforces
-- follows.toggle, so regular members got 403 on every follow/unfollow
-- (audit fix, §31 follow system).

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'pro_member', 'business_member')
  AND p.key = 'follows.toggle'
ON CONFLICT DO NOTHING;

COMMIT;
