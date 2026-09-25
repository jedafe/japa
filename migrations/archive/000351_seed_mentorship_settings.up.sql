-- 000351_seed_mentorship_settings.up.sql
-- Seeds system_settings and comparison type for Mentorship Matching (§102, ADR-110).
--
-- Keys:
--   mentorship.enabled       — feature toggle (default false)
--   mentorship.comparison    — comparison UI toggle (default false)
--   mentorship.max_capacity  — platform-wide cap on mentor_capacity (default 10)
--
-- Also seeds the 'mentors' comparison type in comparison_type_definitions
-- so /api/v1/compare/mentors works via the §30 Generic Comparison System.
--
-- Ref: japaconnect-engineering/sections/102-mentorship-matching.md
--      japaconnect-engineering/sections/30-comparison-system.md

BEGIN;

-- =====================================================================
-- 1. system_settings
--    Note: mentorship.enabled and mentorship.comparison are feature flags
--    (feature_flags table, seeded from the features.Registry), NOT
--    system_settings. Only mentorship.max_capacity is a system_setting.
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'mentorship.max_capacity', '10')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- 2. Comparison type for mentors (§30 integration).
--    Reuses the field_group from the existing 'universities' system type
--    (same pattern as cost_of_living in migration 000248).
-- =====================================================================
INSERT INTO comparison_type_definitions (tenant_id, slug, name, item_label, description, field_group_id, is_system, config)
SELECT
    '00000000-0000-0000-0000-000000000001',
    'mentors',
    'Mentors',
    'Mentor',
    'Compare community mentors across key dimensions',
    field_group_id,
    true,
    '{"fields": ["mentor_areas", "active_mentee_count", "available_slots", "mentor_since", "destination_country", "mentor_bio"]}'::jsonb
FROM comparison_type_definitions
WHERE slug = 'universities' AND tenant_id = '00000000-0000-0000-0000-000000000001'
LIMIT 1
ON CONFLICT (tenant_id, slug) DO NOTHING;

COMMIT;
