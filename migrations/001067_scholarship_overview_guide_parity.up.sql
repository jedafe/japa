-- 001067_scholarship_overview_guide_parity.up.sql
-- Bring wiki_scholarship_overview to parity with the guide vocabulary
-- (guide_scholarship_fields — the group 'scholarships' comparisons now
-- generate dimensions from, 001065). After 001066, scholarship wiki ops and
-- guide edits share the 'content' cfv bucket, so overview fields keyed like
-- the guide set write the same keys the dims hydrate — wiki overview edits
-- populate comparison cells.
--
-- Three near-matching keys are renamed to the guide keys (cfv-safe: values
-- reference field_id, and these fields have zero cfv rows today); five
-- fields are added to complete the 10-field set. Positions mirror the guide
-- group's ordering. All wiki_scholarship_* fields have zero cfv rows in
-- dev and the other groups share no keys with the guide vocabulary.

BEGIN;

-- 1. Rename the three near-matching keys.
UPDATE custom_fields f
SET key = 'award_amount_usd'
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key = 'award_amount';

UPDATE custom_fields f
SET key = 'eligibility_criteria'
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key = 'eligibility';

UPDATE custom_fields f
SET key = 'application_deadline'
FROM custom_field_groups g
WHERE f.group_id = g.id AND g.slug = 'wiki_scholarship_overview'
  AND f.key = 'deadline';

-- 2. Upsert the full guide-key set — labels, types, options and positions
--    mirror guide_scholarship_fields.
INSERT INTO custom_fields
    (group_id, key, label, field_type, options, is_required, is_system,
     show_in_frontend, filterable, sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb,
       false, false, true, false, false, false, m.position, NULL
FROM custom_field_groups g
JOIN (VALUES
    ('scholarship_name',      'Scholarship Name',      'text',     NULL, 1),
    ('provider_institution',  'Provider/Institution',  'text',     NULL, 2),
    ('eligibility_criteria',  'Eligibility Criteria',  'textarea', NULL, 3),
    ('award_amount_usd',      'Award Amount (USD)',    'number',   NULL, 4),
    ('application_deadline',  'Application Deadline',  'date',     NULL, 5),
    ('application_link',      'Application Link',      'url',      NULL, 6),
    ('level_of_study',        'Level of Study',        'select',   '["Undergraduate","Masters","PhD","Postdoc","Any"]', 7),
    ('is_renewable',          'Renewable',             'boolean',  NULL, 8),
    ('coverage',              'Coverage',              'select',   '["Full Tuition","Partial Tuition","Tuition + Stipend","Stipend Only","Research Grant"]', 9),
    ('countries_eligible',    'Countries Eligible',    'repeater', NULL, 10)
) AS m(key, label, field_type, options, position) ON true
WHERE g.slug = 'wiki_scholarship_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, position = EXCLUDED.position;

-- 3. Permissions for the added fields — view all, edit non-guest.
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id AND g.slug = 'wiki_scholarship_overview'
CROSS JOIN user_groups ug
WHERE f.key IN ('scholarship_name', 'provider_institution',
    'level_of_study', 'is_renewable', 'countries_eligible')
ON CONFLICT (field_id, user_group_id) DO NOTHING;

COMMIT;
