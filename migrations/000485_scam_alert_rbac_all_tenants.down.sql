-- 000485_scam_alert_rbac_all_tenants.down.sql
-- Reverses 000485: removes the cross-tenant grants added for scam_alert.{create,review,pin}.
-- Note: this does NOT restore the original (buggy) tenant-1-only super_admin
-- grants — those rows from 000305 remain. This down migration only removes
-- the rows that 000485 inserted.

BEGIN;

-- Remove scam_alert.create grants from all roles (000305's tenant-1 grants
-- are unaffected because they were inserted by a different migration and
-- ON CONFLICT DO NOTHING in 000485 would have skipped re-inserting them).
DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key = 'scam_alert.create'
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp2
      JOIN user_roles ur ON ur.id = rp2.role_id
      WHERE rp2.permission_id = p.id
        AND rp2.role_id = rp.role_id
        AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  );

-- Remove scam_alert.review grants from moderator + admin (super_admin kept).
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'scam_alert.review'
  AND ur.slug IN ('moderator', 'admin');

-- Remove scam_alert.pin grants from admin (super_admin kept).
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'scam_alert.pin'
  AND ur.slug = 'admin';

COMMIT;
