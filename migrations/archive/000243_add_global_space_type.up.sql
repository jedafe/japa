-- Add "Global" space type for spaces that are not tied to a specific geo location.
-- Ref: japaconnect-engineering/sections/05-geo-first-space-system.md
-- Global spaces (e.g. "Remote Workers", "Open Source Contributors") don't require
-- a country→state→city parent. They exist at the platform level.
-- Idempotent: ON CONFLICT DO NOTHING so re-running is safe.

INSERT INTO space_types (id, tenant_id, name, slug, description, icon, field_group_id, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000065',
    '00000000-0000-0000-0000-000000000001',
    'Global',
    'global',
    'Global space not tied to a specific geo location',
    'globe',
    '00000000-0000-0000-0000-000000000043',
    true
)
ON CONFLICT (tenant_id, slug) DO NOTHING;
