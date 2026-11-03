-- 001018_grant_follows_view.up.sql
-- Grants the follows.view node to the member-tier roles. The node was
-- seeded by migration 000109 but never granted, while 000618 granted
-- follows.toggle — meanwhile GET /users/{id}/follow-status, /me/following,
-- /me/followers, /me/followed-tags and /users/{id}/mutual/{other_id} all
-- enforce follows.view, so regular members got 403 on every follow-state
-- read. The SSR profile page degrades that to "not following", which is
-- why the Follow button reset to "Follow" on every refresh even though
-- the follow row was written (§31 follow system, same class of bug as
-- 000618).

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug IN ('member', 'pro_member', 'business_member')
  AND p.key = 'follows.view'
ON CONFLICT DO NOTHING;

COMMIT;
