-- 000454_seed_attestation_guide_feature_flag.down.sql
-- Revert BUG-001 fix: remove the attestation_guide.enabled feature_flags seed.
-- Note: this does NOT re-create the stale feature.attestation_guide row —
-- that row never existed in the first place.

BEGIN;

DELETE FROM feature_flags
WHERE key = 'attestation_guide.enabled'
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

COMMIT;
