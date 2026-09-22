-- 000718_intel_inbound_retention.down.sql — remove the retention seed row.

BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'intel.inbound.retention_days'
);

COMMIT;
