-- Reverts the RBAC permission reconciliation (AUDIT-BE-006).
BEGIN;
DELETE FROM role_permissions rp
USING user_roles ur
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member';
COMMIT;
