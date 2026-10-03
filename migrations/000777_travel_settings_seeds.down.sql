BEGIN;
DELETE FROM system_settings WHERE key IN ('travel.results_ttl_seconds', 'travel.results_retention_days');
COMMIT;
