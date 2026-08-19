-- 000521_credential_pathways_intel_provenance.down.sql
-- Reverses 000521: removes the intelligence provenance columns and index
-- from credential_pathways, and removes the seeded system settings.

BEGIN;

DROP INDEX IF EXISTS idx_credential_pathways_intel_record;

ALTER TABLE credential_pathways DROP COLUMN IF EXISTS editorial_overrides;
ALTER TABLE credential_pathways DROP COLUMN IF EXISTS override_source;
ALTER TABLE credential_pathways DROP COLUMN IF EXISTS freshness_score;
ALTER TABLE credential_pathways DROP COLUMN IF EXISTS verification_status;
ALTER TABLE credential_pathways DROP COLUMN IF EXISTS confidence_overall;
ALTER TABLE credential_pathways DROP COLUMN IF EXISTS guide_id;
ALTER TABLE credential_pathways DROP COLUMN IF EXISTS intel_record_id;

DELETE FROM system_settings WHERE key IN ('intel.credential.enabled', 'intel.credential.freshness_decay_policy');

COMMIT;
