-- 000303_seed_wiki_settings.up.sql
-- Seeds system_settings for the Destination Wiki (§80, ADR-088).
--
-- Keys:
--   wiki.enabled                — feature toggle (default false)
--   wiki.community_edits        — allow community edits (default true when enabled)
--   wiki.min_reputation_to_edit — reputation score required to submit an edit
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md
--      japaconnect-engineering/conventions/system-settings-catalog.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'wiki.enabled',                'false'),
    ('00000000-0000-0000-0000-000000000001', 'wiki.community_edits',        'true'),
    ('00000000-0000-0000-0000-000000000001', 'wiki.min_reputation_to_edit', '100')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
