-- 000392_revoke_community_locator_claim_from_member.down.sql
-- Re-grant community_locator.claim to member (reverts the BUG-015 fix).
-- This restores the original incorrect 000344 seed state.

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'community_locator.claim'
ON CONFLICT DO NOTHING;

COMMIT;
