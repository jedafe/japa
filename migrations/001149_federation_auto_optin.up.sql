-- 001149_federation_auto_optin.up.sql
-- Seed federation.auto_optin so the key is editable from Control → Settings →
-- Federation. true = every new member gets a federated identity provisioned
-- at registration (AP actor under feature.federation; atproto account under
-- feature.atproto); false restores explicit member opt-in (§126 auto opt-in).
-- Not is_public — this is an operational switch, not a site-rendering value.
--
-- Ref: .jeos/sections/126-activitypub-federation.md (auto opt-in)
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (key, value, is_public) VALUES
    ('federation.auto_optin', to_jsonb(true), false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
