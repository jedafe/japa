-- 000288_seed_university_field_definitions.up.sql
-- Seed the individual custom_fields definitions for the university_fields group
-- (group ID 00000000-0000-0000-0000-000000000044, created in migration 000008).
--
-- Ref: japaconnect-engineering/sections/39-space-types-university.md
--
-- The university_fields GROUP was seeded in 000008 but the individual FIELD
-- definitions (official_website_url, world_ranking, founded_year, etc.) were
-- never created. Without these rows, custom_field_values cannot be populated
-- for universities — there is no field_id to reference.
--
-- Fields seeded per Section 39 (excluding inherited Space fields: name,
-- country, state, city; and media fields logo/feature_image which are handled
-- via spaces.logo_media_id and a separate media field, not text values):
--
--   initials              — text (filterable)
--   official_website_url  — url
--   contact_email         — email
--   accreditation_body    — text
--   world_ranking         — number (filterable, sortable)
--   country_ranking       — number (filterable, sortable)
--   founded_year          — number
--   type                  — select ('public' | 'private')
--   programs_offered      — multiselect
--   tuition_range         — select
--   student_population    — number
--   acceptance_rate       — number
--   international_support — boolean
--   scholarship_available — boolean
--   application_url       — url
--
-- Idempotent: ON CONFLICT DO NOTHING on (group_id, key).
-- UUIDs follow the zeros-pattern: 044 is the group, fields start at 090.

BEGIN;

INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, is_system, is_required, show_in_frontend, filterable, sortable, searchable, position) VALUES
    -- initials
    ('00000000-0000-0000-0000-000000000090',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Initials', 'initials', 'text', NULL,
     true, false, true, true, false, true, 0),

    -- official_website_url
    ('00000000-0000-0000-0000-000000000091',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Official Website', 'official_website_url', 'url', NULL,
     true, false, true, false, false, false, 1),

    -- contact_email
    ('00000000-0000-0000-0000-000000000092',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Contact Email', 'contact_email', 'email', NULL,
     true, false, true, false, false, false, 2),

    -- accreditation_body
    ('00000000-0000-0000-0000-000000000093',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Accreditation Body', 'accreditation_body', 'text', NULL,
     true, false, true, false, false, true, 3),

    -- world_ranking
    ('00000000-0000-0000-0000-000000000094',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'World Ranking', 'world_ranking', 'number', NULL,
     true, false, true, true, true, false, 4),

    -- country_ranking
    ('00000000-0000-0000-0000-000000000095',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Country Ranking', 'country_ranking', 'number', NULL,
     true, false, true, true, true, false, 5),

    -- founded_year
    ('00000000-0000-0000-0000-000000000096',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Founded Year', 'founded_year', 'number', NULL,
     true, false, true, false, true, false, 6),

    -- type (public/private)
    ('00000000-0000-0000-0000-000000000097',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Type', 'type', 'select',
     '[{"value":"public","label":"Public"},{"value":"private","label":"Private"}]'::jsonb,
     true, false, true, true, false, false, 7),

    -- programs_offered (multiselect — admin-configurable options, starts empty)
    ('00000000-0000-0000-0000-000000000098',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Programs Offered', 'programs_offered', 'multiselect', '[]'::jsonb,
     true, false, true, true, false, false, 8),

    -- tuition_range
    ('00000000-0000-0000-0000-000000000099',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Tuition Range', 'tuition_range', 'select',
     '[{"value":"free","label":"Free"},{"value":"<5k","label":"< $5k"},{"value":"5k-15k","label":"$5k - $15k"},{"value":"15k-30k","label":"$15k - $30k"},{"value":">30k","label":"> $30k"}]'::jsonb,
     true, false, true, true, false, false, 9),

    -- student_population
    ('00000000-0000-0000-0000-000000000100',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Student Population', 'student_population', 'number', NULL,
     true, false, true, false, true, false, 10),

    -- acceptance_rate
    ('00000000-0000-0000-0000-000000000101',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Acceptance Rate (%)', 'acceptance_rate', 'number', NULL,
     true, false, true, false, true, false, 11),

    -- international_support
    ('00000000-0000-0000-0000-000000000102',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'International Student Support', 'international_support', 'boolean', NULL,
     true, false, true, true, false, false, 12),

    -- scholarship_available
    ('00000000-0000-0000-0000-000000000103',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Scholarship Available', 'scholarship_available', 'boolean', NULL,
     true, false, true, true, false, false, 13),

    -- application_url
    ('00000000-0000-0000-0000-000000000104',
     '00000000-0000-0000-0000-000000000044',
     '00000000-0000-0000-0000-000000000001',
     'Application URL', 'application_url', 'url', NULL,
     true, false, true, false, false, false, 14)
ON CONFLICT (group_id, key) DO NOTHING;

-- Attach the university_fields group to the 'space' entity class so
-- custom_field_values with entity_type='space' can reference these fields.
-- (The group was created in 000008 but never assigned to an entity class.)
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT '00000000-0000-0000-0000-000000000044', 'space', NULL, 0
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments
    WHERE group_id = '00000000-0000-0000-0000-000000000044'
      AND entity_class = 'space'
      AND entity_id IS NULL
);

COMMIT;
