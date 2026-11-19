-- 001212_federation_min_posts_discoverable.up.sql
-- §126: published-post floor (default 3) before a Person or Group actor
-- document reports `discoverable: true` to fediverse directories.
-- Ref: .jeos/sections/126-activitypub-federation.md
-- Ref: .jeos/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (key, value, is_public) VALUES
    ('federation.min_posts_discoverable', to_jsonb(3), false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
