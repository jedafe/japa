-- 001165_federation_settings_seed.up.sql
-- Seed the remaining §126 federation settings so every catalogued key is
-- visible/editable in Control → Settings → Federation. auto_optin was
-- seeded in 001149; these three were catalogued but never seeded.
--
--   federation.outbox_page_size      — items per OrderedCollectionPage
--   federation.delivery.max_attempts — delivery attempts before dead (M2)
--   federation.instances.mode        — open | allowlist (M2 inbound gate)
--
-- Ref: .jeos/sections/126-activitypub-federation.md
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (key, value, is_public) VALUES
    ('federation.outbox_page_size',      to_jsonb(20),      false),
    ('federation.delivery.max_attempts', to_jsonb(8),       false),
    ('federation.instances.mode',        to_jsonb('open'::text), false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
