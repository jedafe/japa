-- 000230_ai_rbac_reconciliation.down.sql
-- Reverses 000230: re-grants all ai.context.* nodes to super_admin_role
-- (restoring the pre-fix 000109 state) and removes the two seeded nodes.
-- NOTE: restoring the bulk grant re-introduces the §19 violation; this down
-- exists for migration reversibility, not for re-enabling the policy bug.

BEGIN;

-- Re-grant all ai.context.* nodes to super_admin (revert step 2).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key LIKE 'ai.context.%'
ON CONFLICT DO NOTHING;

-- Remove ai.editor.assist grant from super_admin (revert step 4).
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND rp.permission_id = p.id
  AND p.key = 'ai.editor.assist';

-- Drop the two seeded nodes (revert step 1). This also cascades any grants.
DELETE FROM permissions WHERE key IN ('ai.context.use', 'ai.editor.assist');

COMMIT;
