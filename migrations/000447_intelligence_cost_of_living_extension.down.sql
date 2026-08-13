-- 000447_intelligence_cost_of_living_extension.down.sql
BEGIN;

ALTER TABLE comparison_items DROP COLUMN IF EXISTS editorial_overrides;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS override_source;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS freshness_score;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS verification_status;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS confidence_overall;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS intel_cost_data;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS intel_record_id;

DROP INDEX IF EXISTS idx_comparison_items_intel_record;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('intel.cost_of_living.enabled', 'intel.cost_of_living.freshness_decay_policy');

COMMIT;
