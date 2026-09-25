-- 000393_seed_coupons_view_rbac_node.up.sql
-- Seeds the missing coupons.view RBAC node (Section 55 — "browse public coupons
-- (default: all)"). The public coupon routes do not enforce this node (they are
-- public), but the node must exist per spec for catalog completeness (297 nodes
-- total) and for any future gating (e.g. restricting coupon browsing to
-- authenticated members).
-- Ref: japaconnect-engineering/sections/55-coupons-external-affiliates.md (RBAC Permission Nodes)

BEGIN;

-- 1. Insert the coupons.view permission node.
INSERT INTO permissions (key, description) VALUES
    ('coupons.view', 'Browse public coupons (default: all)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant to super_admin_role (idempotent) so the super-admin has the node
--    without relying solely on the wildcard '*' bypass.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key = 'coupons.view'
ON CONFLICT DO NOTHING;

COMMIT;
