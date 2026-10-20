-- 000903_wiki_entity_overview_fields.up.sql
-- JCE ↔ Wiki field-group mapping, phase M3b (baseline field schemas):
-- spec docs/superpowers/specs/2026-10-20-jce-wiki-field-group-mapping-design.md
--
-- 000901 created the wiki_{type}_{category} groups as empty schema
-- containers. This migration seeds a baseline field schema into each new
-- type's Overview group so the groups function as comparison dimensions and
-- wiki pages render a fields block immediately. Schemas are deliberately
-- small (4–6 keys) — community edits and AI field ops grow them per §80.

BEGIN;

-- Visa routes — base type + all six kinds share the baseline schema
-- (per-kind groups stay independent so kinds can diverge, e.g. PR pathway
-- gaining residency-specific fields without touching tourist visas).
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT g.id, v.label, v.key, v.field_type, NULL::jsonb, false, true, true, v.filterable, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('Route Type',       'route_type',        'text',     true,  1),
    ('Processing Time',  'processing_time',   'text',     true,  2),
    ('Application Fee',  'application_fee',   'number',   true,  3),
    ('Eligibility',      'eligibility',       'textarea', false, 4),
    ('Official Link',    'official_link',     'url',      false, 5)
) AS v(label, key, field_type, filterable, position)
WHERE g.slug IN (
    'wiki_visa_route_overview',
    'wiki_student_visa_overview',
    'wiki_work_visa_overview',
    'wiki_family_visa_overview',
    'wiki_tourist_visa_overview',
    'wiki_business_visa_overview',
    'wiki_pr_pathway_overview'
)
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

-- Course overview.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT g.id, v.label, v.key, v.field_type, NULL::jsonb, false, true, true, v.filterable, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('Degree Type',         'degree_type',         'text',     true,  1),
    ('Duration',            'duration',            'text',     true,  2),
    ('Tuition Note',        'tuition_note',        'text',     true,  3),
    ('Entry Requirements',  'entry_requirements',  'textarea', false, 4),
    ('Career Outcomes',     'career_outcomes',     'textarea', false, 5)
) AS v(label, key, field_type, filterable, position)
WHERE g.slug = 'wiki_course_overview'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

-- Scholarship overview.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT g.id, v.label, v.key, v.field_type, NULL::jsonb, false, true, true, v.filterable, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('Award Amount',     'award_amount',     'text',     true,  1),
    ('Coverage',         'coverage',         'text',     true,  2),
    ('Eligibility',      'eligibility',      'textarea', false, 3),
    ('Deadline',         'deadline',         'text',     true,  4),
    ('Application Link', 'application_link', 'url',      false, 5)
) AS v(label, key, field_type, filterable, position)
WHERE g.slug = 'wiki_scholarship_overview'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

-- Bank overview.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT g.id, v.label, v.key, v.field_type, NULL::jsonb, false, true, true, v.filterable, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('Bank Type',              'bank_type',              'text',  true,  1),
    ('Account Requirements',   'account_requirements',   'text',  true,  2),
    ('Maintenance Fees',       'maintenance_fees',       'text',  true,  3),
    ('International Features', 'international_features', 'text',  false, 4),
    ('Website',                'website',                'url',   false, 5)
) AS v(label, key, field_type, filterable, position)
WHERE g.slug = 'wiki_bank_overview'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

-- Embassy overview.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT g.id, v.label, v.key, v.field_type, NULL::jsonb, false, true, true, v.filterable, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('Services',         'services',         'textarea', true,  1),
    ('Appointment Link', 'appointment_link', 'url',      true,  2),
    ('Address',          'address',          'text',     false, 3),
    ('Phone',            'phone',            'text',     false, 4)
) AS v(label, key, field_type, filterable, position)
WHERE g.slug = 'wiki_embassy_overview'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

-- Neighborhood overview.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config)
SELECT g.id, v.label, v.key, v.field_type, NULL::jsonb, false, true, true, v.filterable, false, true, v.position, '{}'::jsonb
FROM custom_field_groups g
CROSS JOIN (VALUES
    ('Character',     'character',     'text',  true,  1),
    ('Safety Note',   'safety_note',   'text',  true,  2),
    ('Typical Rent',  'typical_rent',  'text',  true,  3),
    ('Walkability',   'walkability',   'text',  false, 4)
) AS v(label, key, field_type, filterable, position)
WHERE g.slug = 'wiki_neighborhood_overview'
  AND NOT EXISTS (
      SELECT 1 FROM custom_fields cf WHERE cf.group_id = g.id AND cf.key = v.key
  );

COMMIT;
