-- 000694_seed_attestation_guide_feature_flag.down.sql
-- Renumbered from 000454 (duplicate version prefix). See the .up file.
-- 000454_seed_attestation_guide_feature_flag.down.sql
-- Revert BUG-001 fix: remove the attestation_guide.enabled feature_flags seed.
-- Note: this does NOT re-create the stale feature.attestation_guide row —
-- that row never existed in the first place.
-- De-tenanted form: feature_flags.tenant_id was dropped by 000668, so the
-- original tenant filter is gone.

BEGIN;

DELETE FROM feature_flags
WHERE key = 'attestation_guide.enabled';

COMMIT;
