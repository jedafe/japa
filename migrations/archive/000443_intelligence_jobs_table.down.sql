-- 000443_intelligence_jobs_table.down.sql
BEGIN;

DROP TABLE IF EXISTS intel_jobs CASCADE;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('intel.jobs.enabled', 'intel.jobs.freshness_decay_policy');

COMMIT;
