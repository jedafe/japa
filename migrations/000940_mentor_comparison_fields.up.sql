-- 000940_mentor_comparison_fields.up.sql
-- Give the 'mentors' comparison type its own field group. Migration 000351
-- seeded the type with the universities field_group_id, so every mentors
-- quick comparison auto-generated university dimensions (Initials,
-- Official Website, World Ranking, ...). The new group mirrors the §114
-- "Mentorship details" sidebar (session type, focus areas, countries,
-- expertise, experience, mentees helped, success rate, response time,
-- availability, languages, formats, location, package price) plus the
-- §102 comparison_fields set; the mentor JCE resolver projects the same
-- keys from user_profiles/shadow-listing data into item field_values so
-- matrix cells hydrate.
--
-- package_price is text, not currency: currency dimensions take their code
-- from the field's options (one fixed code), but mentors price their
-- shadow listings in mixed currencies — the resolver emits a formatted
-- string instead.
--
-- Ref: .jeos/sections/102-mentorship-matching.md (§30 integration table)
--      .jeos/sections/114-services-marketplace.md (mentor card fields)
--      .jeos/sections/30-generic-comparison-system.md (Sub-spec 5)

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. Mentor Fields group (system-owned; not assigned to any profile surface —
--    it exists purely to drive §30 dimension auto-generation).
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_groups (id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000710',
    'Mentor Fields',
    'mentor_fields',
    'Comparison dimensions for the mentors comparison type (§102/§114)',
    true
)
ON CONFLICT (slug) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2. Fields mirroring the "Mentorship details" sidebar, then the §102
--    comparison_fields extras. Field types map to comparison dimension types
--    via fieldTypeToDimensionType: multiselect/textarea/text → text,
--    number → number, date → text.
-- ---------------------------------------------------------------------------
INSERT INTO custom_fields (id, group_id, label, key, field_type, options,
                           is_required, is_system, show_in_frontend,
                           filterable, sortable, searchable, position)
SELECT v.id::uuid, g.id, v.label, v.key, v.field_type, '[]'::jsonb,
       false, true, false, false, false, false, v.position
FROM (VALUES
    ('00000000-0000-0000-0000-000000000711', 'Session type',       'mentor_session_type',     'text',        0),
    ('00000000-0000-0000-0000-000000000712', 'Focus areas',        'mentor_areas',           'multiselect', 1),
    ('00000000-0000-0000-0000-000000000713', 'Countries covered',  'mentor_countries',       'multiselect', 2),
    ('00000000-0000-0000-0000-000000000714', 'Expertise',          'mentor_credentials',     'multiselect', 3),
    ('00000000-0000-0000-0000-000000000715', 'Experience',         'mentor_years_experience','number',      4),
    ('00000000-0000-0000-0000-000000000716', 'Mentees helped',     'mentees_helped',         'number',      5),
    ('00000000-0000-0000-0000-000000000717', 'Success rate',       'mentor_success_label',   'text',        6),
    ('00000000-0000-0000-0000-000000000718', 'Response time',      'mentor_response_time',   'text',        7),
    ('00000000-0000-0000-0000-000000000719', 'Availability',       'mentor_availability',    'text',        8),
    ('00000000-0000-0000-0000-00000000071a', 'Languages',          'mentor_languages',       'multiselect', 9),
    ('00000000-0000-0000-0000-00000000071b', 'Formats',            'mentor_session_formats', 'multiselect', 10),
    ('00000000-0000-0000-0000-00000000071c', 'Location',           'location',               'text',        11),
    ('00000000-0000-0000-0000-00000000071d', 'Package price',      'package_price',          'text',        12),
    ('00000000-0000-0000-0000-00000000071e', 'Profession',         'profession',             'text',        13),
    ('00000000-0000-0000-0000-00000000071f', 'Headline',           'mentor_title',           'text',        14),
    ('00000000-0000-0000-0000-000000000720', 'Member since',       'mentor_since',           'date',        15),
    ('00000000-0000-0000-0000-000000000721', 'Slots remaining',    'available_slots',        'number',      16),
    ('00000000-0000-0000-0000-000000000722', 'Active mentees',     'active_mentee_count',    'number',      17),
    ('00000000-0000-0000-0000-000000000723', 'Destination',        'destination_country',    'text',        18),
    ('00000000-0000-0000-0000-000000000724', 'Bio',                'mentor_bio',             'textarea',    19)
) AS v(id, label, key, field_type, position)
JOIN custom_field_groups g ON g.slug = 'mentor_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3. Repoint the mentors type at its own group (was university_fields), and
--    drop config.fields — the §102 field list that nothing ever consumed; the
--    field group is now the single source of dimension truth.
-- ---------------------------------------------------------------------------
UPDATE comparison_type_definitions t
SET field_group_id = g.id,
    config = t.config - 'fields'
FROM custom_field_groups g
WHERE g.slug = 'mentor_fields'
  AND t.slug = 'mentors';

-- ---------------------------------------------------------------------------
-- 4. Repair mentors comparisons created while the type was bound to
--    university_fields: soft-delete the stale university dims (persisted cell
--    values stay attached to the deleted dim and simply stop rendering —
--    Rule 11), then re-seed the mentor field set per comparison. Idempotent
--    on re-run via the NOT EXISTS guard.
-- ---------------------------------------------------------------------------
UPDATE comparison_dimensions d
SET deleted_at = now()
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
WHERE d.source_field_id = f.id
  AND g.slug = 'university_fields'
  AND d.deleted_at IS NULL
  AND d.type_id = (SELECT id FROM comparison_type_definitions
                   WHERE slug = 'mentors' AND is_system = true);

INSERT INTO comparison_dimensions (content_id, type_id, label, dimension_type, position, source_field_id)
SELECT DISTINCT mc.content_id, t.id, f.label,
       CASE f.field_type
           WHEN 'number' THEN 'number' WHEN 'integer' THEN 'number'
           WHEN 'currency' THEN 'currency' WHEN 'price_range' THEN 'currency'
           WHEN 'boolean' THEN 'boolean' WHEN 'checkbox' THEN 'boolean'
           WHEN 'rating' THEN 'rating' WHEN 'select' THEN 'rating'
           ELSE 'text'
       END,
       f.position, f.id
FROM (
    SELECT content_id FROM comparison_items
    WHERE type_id = (SELECT id FROM comparison_type_definitions
                     WHERE slug = 'mentors' AND is_system = true)
    UNION
    SELECT content_id FROM comparison_dimensions
    WHERE type_id = (SELECT id FROM comparison_type_definitions
                     WHERE slug = 'mentors' AND is_system = true)
) mc
JOIN comparison_type_definitions t ON t.slug = 'mentors' AND t.is_system = true
JOIN custom_field_groups g ON g.slug = 'mentor_fields'
JOIN custom_fields f ON f.group_id = g.id
WHERE mc.content_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM comparison_dimensions dx
      WHERE dx.content_id = mc.content_id
        AND dx.source_field_id = f.id
        AND dx.deleted_at IS NULL
  );

COMMIT;
