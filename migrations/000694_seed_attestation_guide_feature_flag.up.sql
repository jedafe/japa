-- 000694_seed_attestation_guide_feature_flag.up.sql
-- Renumbered from 000454 — it collided with 000454_appointment_alert_rbac_backfill.
-- De-tenanted form: feature_flags.tenant_id was dropped by 000668 and the flag is
-- now keyed globally on `key`. Tail-position body; databases that applied the
-- 000454 file skip this via the 000000 registry marker.

BEGIN;

INSERT INTO feature_flags (key, enabled) VALUES
    ('attestation_guide.enabled', false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
