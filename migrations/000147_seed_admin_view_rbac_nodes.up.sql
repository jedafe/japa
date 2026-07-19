-- 000147_seed_admin_view_rbac_nodes.up.sql
-- RBAC seed gap fix (Bug Hunter's Guide §26 / §9): the admin frontend references 12
-- "view/aggregate" permission nodes that were never seeded in the backend, so they
-- could not be granted to any role and the frontend RBAC gate denied pages for every
-- non-super-admin role. Migration 000109 seeded the 174 backend-ENFORCED nodes; these
-- 12 are the additional admin "view" nodes the frontend gate checks.
-- Idempotent; safe to re-run. Grants them to the super_admin role(s).

BEGIN;

-- 1. Ensure the 12 admin view permission nodes exist.
INSERT INTO permissions (key, description) VALUES
    ('admin.users.view', 'View users section in admin panel'),
    ('admin.content.view', 'View content section in admin panel'),
    ('admin.moderation.view', 'View moderation section in admin panel'),
    ('admin.analytics.view', 'View analytics section in admin panel'),
    ('admin.settings.view', 'View settings section in admin panel'),
    ('admin.roles.view', 'View roles & permissions section in admin panel'),
    ('admin.audit.view', 'View audit logs section in admin panel'),
    ('admin.advertising.view', 'View advertising section in admin panel'),
    ('admin.payments.view', 'View payments section in admin panel'),
    ('admin.ai.view', 'View AI section in admin panel'),
    ('admin.feature_flags.view', 'View feature flags section in admin panel'),
    ('wallet.admin', 'Access wallet administration')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant the 12 nodes to the super_admin role(s).
-- Both 'super_admin' (master seed 000099 / 000109) and 'super_admin_role'
-- (used by several later seed migrations) are targeted defensively; the SELECT
-- simply returns no rows for whichever slug is absent, so this is safe and idempotent.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'super_admin_role')
  AND p.key IN (
    'admin.users.view',
    'admin.content.view',
    'admin.moderation.view',
    'admin.analytics.view',
    'admin.settings.view',
    'admin.roles.view',
    'admin.audit.view',
    'admin.advertising.view',
    'admin.payments.view',
    'admin.ai.view',
    'admin.feature_flags.view',
    'wallet.admin'
  )
ON CONFLICT DO NOTHING;

COMMIT;
