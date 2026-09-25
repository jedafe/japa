-- 000561_seed_ai_bulk_tools_and_perms.down.sql
-- Reverts the bulk content tool and per-content-type permission additions.

BEGIN;

DELETE FROM ai_tools
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND name IN ('content.bulk_create', 'content.bulk_publish');

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('agent.bulk_create_max_items', 'agent.bulk_publish_max_items');

DELETE FROM role_permissions
WHERE role_id IN (
    SELECT id FROM user_roles
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND slug IN ('member', 'super_admin')
)
  AND permission_id IN (
    SELECT id FROM permissions WHERE key LIKE 'agent.content_type.%'
);

DELETE FROM permissions
WHERE key LIKE 'agent.content_type.%';

COMMIT;
