-- 000444_intelligence_visa_routes_extension.down.sql
BEGIN;

ALTER TABLE visa_routes DROP COLUMN IF EXISTS editorial_overrides;
ALTER TABLE visa_routes DROP COLUMN IF EXISTS override_source;
ALTER TABLE visa_routes DROP COLUMN IF EXISTS freshness_score;
ALTER TABLE visa_routes DROP COLUMN IF EXISTS verification_status;
ALTER TABLE visa_routes DROP COLUMN IF EXISTS confidence_overall;
ALTER TABLE visa_routes DROP COLUMN IF EXISTS intel_record_id;

DROP INDEX IF EXISTS idx_visa_routes_intel_record;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('intel.visa.enabled', 'intel.visa.freshness_decay_policy');

COMMIT;
