-- 001063_service_listing_quota.down.sql

BEGIN;

DELETE FROM system_settings WHERE key LIKE 'services.listings.max_per_user%';

COMMIT;
