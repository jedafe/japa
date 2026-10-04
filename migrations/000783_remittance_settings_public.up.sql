-- 000783_remittance_settings_public.up.sql
-- Expose the remittance settings consumed by public surfaces:
--   remittance.default_send_currency — pre-selected corridor send currency
--     (main + tools frontends, §74).
--   remittance.manual_stale_hours    — drives the "stale" badge on manual
--     rates (tools frontend).
--   remittance.subdomain / remittance.custom_domain — tool host detection in
--     the tools app hooks (same is_public treatment as fx/scholarship_finder).
--
-- Ref: .jeos/conventions/system-settings-catalog.md
--      .jeos/sections/74-remittance-tracker.md

BEGIN;

UPDATE system_settings SET is_public = true
WHERE key IN (
    'remittance.default_send_currency',
    'remittance.manual_stale_hours',
    'remittance.subdomain',
    'remittance.custom_domain'
);

COMMIT;
