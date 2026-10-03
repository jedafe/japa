-- 000780_fx_rates.down.sql
BEGIN;
DELETE FROM system_settings WHERE key IN (
    'fx.cache_ttl_minutes', 'fx.stale_hours', 'fx.history_retention_days',
    'fx.default_base_currency', 'fx.subdomain', 'fx.custom_domain');
DELETE FROM remittance_providers WHERE slug = 'mid-market' AND is_benchmark = true;
ALTER TABLE remittance_providers DROP COLUMN IF EXISTS is_benchmark;
DROP TABLE IF EXISTS fx_rates;
COMMIT;
