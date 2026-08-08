-- 000300_seed_story_settings.up.sql
-- Seeds system_settings for the Community Story Wall (§79, ADR-087).
--
-- Keys:
--   story.enabled       — feature toggle (default false, per §79 feature flags)
--   story.subdomain     — optional standalone subdomain (ADR-086)
--   story.custom_domain — optional custom domain (ADR-086)
--
-- Ref: japaconnect-engineering/sections/79-community-story-wall.md
--      japaconnect-engineering/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'story.enabled',       'false'),
    ('00000000-0000-0000-0000-000000000001', 'story.subdomain',     '""'),
    ('00000000-0000-0000-0000-000000000001', 'story.custom_domain', '""')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
