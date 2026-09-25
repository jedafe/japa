-- 000284_ai_humanization.down.sql
-- Reverses 000284: removes the ai.humanize.* settings and the RBAC node.
-- Only deletes settings rows whose value still matches the seeded default,
-- so any admin override is preserved.

BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('ai.humanize.enabled', 'ai.humanize.level')
  AND value IN ('"false"', '"medium"');

DELETE FROM role_permissions
USING user_roles ur, permissions p
WHERE role_permissions.role_id = ur.id
  AND role_permissions.permission_id = p.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'ai.humanize.manage';

DELETE FROM permissions WHERE key = 'ai.humanize.manage';

COMMIT;
