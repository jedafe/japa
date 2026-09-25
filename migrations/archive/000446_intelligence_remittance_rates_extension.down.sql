-- 000446_intelligence_remittance_rates_extension.down.sql
BEGIN;

ALTER TABLE remittance_rates DROP COLUMN IF EXISTS editorial_overrides;
ALTER TABLE remittance_rates DROP COLUMN IF EXISTS override_source;
ALTER TABLE remittance_rates DROP COLUMN IF EXISTS freshness_score;
ALTER TABLE remittance_rates DROP COLUMN IF EXISTS verification_status;
ALTER TABLE remittance_rates DROP COLUMN IF EXISTS confidence_overall;
ALTER TABLE remittance_rates DROP COLUMN IF EXISTS intel_record_id;

DROP INDEX IF EXISTS idx_remittance_rates_intel_record;

DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('intel.remittance.enabled', 'intel.remittance.freshness_decay_policy');

COMMIT;
