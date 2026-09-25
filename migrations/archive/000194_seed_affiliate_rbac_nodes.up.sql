-- 000194_seed_affiliate_member_rbac_nodes.up.sql
-- Seeds the two member-scope User Affiliate Program RBAC nodes (Section 56)
-- that were not included in the earlier reconciliation migrations
-- (000109/000125 seeded only the affiliate.admin.* nodes).
-- Ref: japaconnect-engineering/sections/56-user-affiliate-program.md (RBAC Permission Nodes)

BEGIN;

-- 1. Insert the 2 member-scope affiliate.* permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('affiliate.program.enroll', 'Enroll in the affiliate program (verified members >= affiliate.min_account_days)'),
    ('affiliate.links.manage',   'Create and manage own affiliate links (granted on enrollment)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant them to super_admin_role (idempotent) so the super-admin can use
--    the member-facing affiliate endpoints without relying solely on the
--    wildcard '*' bypass. Member-role granting is handled by the enrollment
--    flow, not by this seed.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key IN ('affiliate.program.enroll', 'affiliate.links.manage')
ON CONFLICT DO NOTHING;

COMMIT;
