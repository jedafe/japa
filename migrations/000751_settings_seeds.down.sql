-- 000751_settings_seeds.down.sql — remove the seeded system_settings rows.

BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'wiki.rollup_default_rows_limit',
    'intel.rejected_retention_days'
);

COMMIT;
