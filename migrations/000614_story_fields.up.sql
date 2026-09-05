-- Seed the story_fields custom field group for the default tenant so stories
-- authored through the /create composer (content_type='story') collect the
-- §79 structured fields. Values travel in contents.field_values and are
-- synced into story_meta by the contents service.
INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-000000000001',
    'Story Fields',
    'story_fields',
    'Structured fields for community relocation stories (Section 79)',
    true
)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, placeholder, options, position, is_system, show_in_frontend)
SELECT cfg.id,
       '00000000-0000-0000-0000-000000000001',
       label,
       key,
       field_type,
       placeholder,
       options,
       position,
       true,
       true
FROM custom_field_groups cfg
CROSS JOIN (VALUES
    ('Relocation Year',      'relocation_year',      'number',   '2024',                                            NULL::jsonb, 0),
    -- §06 'suggest' fields: type-ahead over options with free-text fallback.
    ('Visa Category',        'visa_category',        'suggest',  'e.g. Skilled Worker',
        '["Student Visa","Work Visa","Skilled Worker Visa","Business Visa","Tourist Visa","Family Visa","Spouse / Partner Visa","Permanent Residency","Transit Visa","Asylum","Other"]'::jsonb, 1),
    ('Profession',           'profession',           'suggest',  'e.g. Software Engineer',
        '["Software Engineer","Nurse","Doctor","Teacher","Accountant","Data Analyst","Product Manager","Designer","Pharmacist","Electrician","Civil Engineer","Care Worker","Chef","Lawyer","Student","Other"]'::jsonb, 2),
    ('Biggest Challenge',    'biggest_challenge',    'textarea', 'What was the hardest part of your journey?',      NULL::jsonb, 3),
    -- 'repeater' — members can list several resources, not just one.
    ('Most Useful Resources','most_useful_resource', 'repeater', 'A website, community, document, etc.',            NULL::jsonb, 4),
    ('Best Advice',          'best_advice',          'textarea', 'What advice would you give to someone starting this journey?', NULL::jsonb, 5)
) AS f(label, key, field_type, placeholder, options, position)
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'story_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
SELECT cfg.id, 'content_type:story', 0
FROM custom_field_groups cfg
WHERE cfg.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND cfg.slug = 'story_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = cfg.id AND a.entity_class = 'content_type:story'
  );
