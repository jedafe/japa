-- 000183_seed_university_revenue_rbac_nodes.up.sql
-- ADR-063 §9: read-only admin revenue-split audit viewer. Seeds the two
-- RBAC nodes that gate GET /admin/university/revenue-splits (list) and
-- GET /admin/university/revenue-splits/{id} (view), and grants them to the
-- super_admin_role so the RBAC CRUD gate does not fire.
-- Ref: japaconnect-engineering/adr/ADR-063-japa-university-creator-economy.md (§9)

BEGIN;

-- 1. Insert the 2 university.revenue.* permission nodes.
INSERT INTO permissions (key, description) VALUES
    ('university.revenue.list', 'List immutable revenue-split audit records (admin audit viewer, ADR-063 §9)'),
    ('university.revenue.view', 'View a single immutable revenue-split audit record by ID (admin audit viewer, ADR-063 §9)')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant both to super_admin_role (idempotent). Super-admin also bypasses
-- RBAC via the wildcard '*' node (Section 03), but explicit grants keep the
-- node visible in role-permission listings and survive a wildcard removal.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin_role'
  AND p.key IN ('university.revenue.list', 'university.revenue.view')
ON CONFLICT DO NOTHING;

COMMIT;
