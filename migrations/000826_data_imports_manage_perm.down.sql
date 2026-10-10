-- 000826_data_imports_manage_perm.down.sql — revoke the
-- data_imports.manage grant from the super_admin roles (restores the
-- pre-migration state where no role held the node). The permission row
-- itself is left in place.

DELETE FROM role_permissions
WHERE permission_id = (SELECT id FROM permissions WHERE key = 'data_imports.manage')
  AND role_id IN (
      SELECT ur.id FROM user_roles ur
      WHERE ur.slug IN ('super_admin', 'super_admin_role')
  );
