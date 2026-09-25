-- 000426_grant_visa_routes_manage_to_admin.up.sql
-- Fix: §90 spec requires visa_routes.manage to be granted to admin AND
-- super_admin, but migration 000326 only granted it to super_admin.
-- This migration adds the missing admin role grant.
--
-- Ref: japaconnect-engineering/sections/90-visa-route-comparison.md

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('admin')
  AND p.key = 'visa_routes.manage'
ON CONFLICT DO NOTHING;

COMMIT;
