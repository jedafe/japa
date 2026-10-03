BEGIN;
DELETE FROM system_settings WHERE key = 'travel.locations_cache_ttl_seconds';
COMMIT;
