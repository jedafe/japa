-- 000367_seed_geo_lookup_permission.up.sql
-- Seed the `geo.lookup` permission node that gates the admin-only
-- GET /api/v1/geo/lookup route (Section 37, ADR-037). Without this node the
-- route is a permanent 403 for any non-super-admin admin role, because
-- RequirePermission checks the node and super_admin is the only role that
-- bypasses the check.
--
-- Ref: japaconnect-engineering/sections/37-ip-geolocation.md
--      japaconnect-engineering/conventions/permission-key-naming.md (IP Geolocation)

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('geo.lookup', 'Geo IP Lookup Arbitrary Address')
ON CONFLICT (key) DO NOTHING;

-- Grant to super_admin (blanket) and admin so the admin panel can resolve
-- arbitrary IPs without granting super_admin. Scoped to the default tenant
-- where the system roles live (migration 000006).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'admin')
  AND p.key = 'geo.lookup'
ON CONFLICT DO NOTHING;

COMMIT;
