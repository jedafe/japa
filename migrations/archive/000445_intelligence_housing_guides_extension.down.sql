-- 000445_intelligence_housing_guides_extension.down.sql
BEGIN;

ALTER TABLE housing_guides DROP COLUMN IF EXISTS editorial_overrides;
ALTER TABLE housing_guides DROP COLUMN IF EXISTS override_source;
ALTER TABLE housing_guides DROP COLUMN IF EXISTS freshness_score;
ALTER TABLE housing_guides DROP COLUMN IF EXISTS verification_status;
ALTER TABLE housing_guides DROP COLUMN IF EXISTS confidence_overall;
ALTER TABLE housing_guides DROP COLUMN IF EXISTS intel_record_id;

DROP INDEX IF EXISTS idx_housing_guides_intel_record;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('intel.housing.enabled', 'intel.housing.freshness_decay_policy');

COMMIT;
