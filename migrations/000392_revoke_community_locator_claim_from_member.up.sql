-- 000392_revoke_community_locator_claim_from_member.up.sql
-- Fix BUG-015: community_locator.claim was incorrectly granted to the
-- `member` role in migration 000344. Per §101 RBAC, claim is for
-- org_member, professional, agent (elevated roles, not member+).
-- Following the mentorship precedent (000350), elevated-role permissions
-- are granted to super_admin as a stand-in. The grant to member is revoked.
--
-- Ref: japaconnect-engineering/sections/101-diaspora-community-locator.md
--      japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'community_locator.claim';

COMMIT;
