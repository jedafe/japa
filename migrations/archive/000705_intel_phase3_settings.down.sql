-- 000705_intel_phase3_settings.down.sql — remove the Phase-3 seed row.

BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'intel.email.signing_key'
);

COMMIT;
