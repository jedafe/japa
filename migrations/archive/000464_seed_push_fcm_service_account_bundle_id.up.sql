-- 000464_seed_push_fcm_service_account_bundle_id.up.sql
-- Seed the push.fcm_service_account and push.apns_bundle_id system_settings
-- for the default tenant. These keys are consumed by internal/devices/push.go
-- (CompositeDispatcher) for FCM HTTP v1 API and APNs topic header.
--
-- push.fcm_service_account replaces the deprecated push.fcm_server_key
-- (Google shut down the legacy FCM server key API in June 2024).
-- push.apns_bundle_id replaces the previously hardcoded apns-topic header
-- (Rule 3: no hardcoded config values).
--
-- Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
--      japaconnect-engineering/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'push.fcm_service_account', '""'),
    ('00000000-0000-0000-0000-000000000001', 'push.apns_bundle_id',      '""')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
