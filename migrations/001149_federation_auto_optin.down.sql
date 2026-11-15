-- 001149_federation_auto_optin.down.sql
-- Remove the seeded federation.auto_optin key. The compiled-in default
-- (auto-provision on) still applies when the row is absent.

BEGIN;

DELETE FROM system_settings WHERE key = 'federation.auto_optin';

COMMIT;
