-- 000435_intel_sources_view_permission.up.sql
-- Adds the missing intel.sources.view permission node that the Control Panel
-- "Data Intelligence" section checks (apps/control /intelligence +page.svelte:27,44
-- and /intelligence/sources +page.svelte:44). Without this node no role —
-- including super_admin — can hold the grant, so the entire intelligence
-- moderation surface is permanently access-denied. BUG-007 fix.
-- Ref: ADR-112, docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md §6

BEGIN;

-- 1. Insert the intel.sources.view permission node (idempotent).
INSERT INTO permissions (key, description) VALUES
    ('intel.sources.view', 'Intel Sources View')
ON CONFLICT (key) DO NOTHING;

-- 2. Grant intel.sources.view to super_admin and member roles on the default
--    tenant so admins can manage sources and any authenticated user can view
--    the public source directory. platform_admin is covered via super_admin
--    inheritance; member gets view-only access to the directory listing.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND p.key = 'intel.sources.view'
ON CONFLICT DO NOTHING;

COMMIT;
