-- 000451_seed_affiliate_member_permissions.up.sql
-- Grants affiliate.program.enroll and affiliate.links.manage to the members
-- group's member_role so verified members can access the affiliate program
-- endpoints (Section 56 RBAC). The service layer enforces actual enrollment
-- (CreateLink/GetMember checks); the RBAC gate just ensures the user is a
-- member. This fixes the infinite enrollment loop where Enroll() created the
-- affiliate_program_members row but never granted the permission, so users
-- got 403 on /me/affiliate-links even after enrolling (BUG-002).
--
-- The user_role_assignments table (spec §03) was never created, so per-user
-- "granted on enrollment" role assignment is impossible. Granting to the
-- members group's role is the correct V1 approach — the service layer's
-- enrollment check is the real gate.
--
-- Ref: japaconnect-engineering/sections/56-user-affiliate-program.md (RBAC)

BEGIN;

-- Grant affiliate.program.enroll and affiliate.links.manage to member_role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
JOIN permissions p ON p.key IN ('affiliate.program.enroll', 'affiliate.links.manage')
WHERE r.slug = 'member_role'
ON CONFLICT DO NOTHING;

COMMIT;
