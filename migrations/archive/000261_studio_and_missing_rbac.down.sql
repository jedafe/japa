-- 000261_studio_and_missing_rbac.down.sql
-- Rollback: drop studio tables, remove RBAC nodes and system_settings.
-- Ref: japaconnect-engineering/sections/75-ai-content-studio.md, 76-participation-orchestration.md

BEGIN;

-- Remove role_permissions grants for the new nodes.
DELETE FROM role_permissions
WHERE permission_id IN (
  SELECT id FROM permissions
  WHERE key IN (
    'studio.manage', 'studio.create', 'studio.review', 'studio.view',
    'participation.manage', 'participation.review', 'participation.view',
    'devices.manage', 'devices.admin'
  )
);

-- Remove the permission nodes.
DELETE FROM permissions
WHERE key IN (
  'studio.manage', 'studio.create', 'studio.review', 'studio.view',
  'participation.manage', 'participation.review', 'participation.view',
  'devices.manage', 'devices.admin'
);

-- Remove system_settings.
DELETE FROM system_settings
WHERE key IN (
  'studio.enabled', 'studio.max_concurrent_jobs', 'studio.auto_publish_types',
  'studio.default_persona_id',
  'participation.enabled', 'participation.run_interval_minutes',
  'participation.global_daily_cap'
)
AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- Drop studio tables (jobs first due to FK to batches).
DROP TABLE IF EXISTS studio_jobs;
DROP TABLE IF EXISTS studio_batches;

COMMIT;
