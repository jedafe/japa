-- 000472_seed_studio_auto_promote_intel_records.down.sql
BEGIN;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'studio.auto_promote_intel_records';

COMMIT;
