-- 000579_jce_subspec4_member_perms.up.sql
-- Grant consumer-facing JCE Sub-spec 4 permissions to the member role so the
-- apps/jce UI (refresh button, AI advisor) is usable by regular users.
-- Ref: .jeos/audit/jce-audit.md JCE-2026-08-31-002

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN ('jce.refresh.trigger', 'jce.ai_advisor.use')
ON CONFLICT DO NOTHING;

COMMIT;
