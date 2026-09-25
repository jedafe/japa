-- 000491_wiki_rbac_all_tenants.down.sql
-- Reverses 000491: removes the cross-tenant grants added for
-- wiki.{edit,review,manage,lock}. Note: this does NOT restore the original
-- (buggy) tenant-1-only super_admin grants from 000302 — those rows remain.
-- This down migration only removes the rows that 000491 inserted.

BEGIN;

-- Remove wiki.edit grants from all roles in all tenants (000302's
-- tenant-1 member+super_admin grants are unaffected).
DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key = 'wiki.edit'
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp2
      JOIN user_roles ur ON ur.id = rp2.role_id
      WHERE rp2.permission_id = p.id
        AND rp2.role_id = rp.role_id
        AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
        AND ur.slug IN ('member', 'super_admin')
  );

-- Remove wiki.review grants from moderator/admin in all tenants.
DELETE FROM role_permissions rp
USING permissions p, user_roles ur
WHERE rp.permission_id = p.id
  AND rp.role_id = ur.id
  AND p.key = 'wiki.review'
  AND ur.slug IN ('moderator', 'admin');

-- Remove wiki.manage grants from admin in all tenants (super_admin kept
-- from 000302).
DELETE FROM role_permissions rp
USING permissions p, user_roles ur
WHERE rp.permission_id = p.id
  AND rp.role_id = ur.id
  AND p.key = 'wiki.manage'
  AND ur.slug = 'admin';

-- Remove wiki.lock grants from admin in all tenants (super_admin kept
-- from 000302).
DELETE FROM role_permissions rp
USING permissions p, user_roles ur
WHERE rp.permission_id = p.id
  AND rp.role_id = ur.id
  AND p.key = 'wiki.lock'
  AND ur.slug = 'admin';

COMMIT;
