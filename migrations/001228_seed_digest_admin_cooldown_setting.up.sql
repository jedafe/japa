-- 001228_seed_digest_admin_cooldown_setting.up.sql
-- Seeds the §84 per-admin generation cooldown window. The service falls
-- back to 300s when unset; seeding makes it visible/administrable in
-- Control → Settings → Digest. 0 disables the cooldown.
--
-- Ref: japaconnect-engineering/sections/84-community-digest-widget.md
-- Ref: japaconnect-engineering/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('digest.admin_generate_cooldown_seconds', '300')
ON CONFLICT (key) DO NOTHING;

COMMIT;
