-- Add category to space_types and seed the new category system types.
-- Ref: docs/superpowers/specs/2026-08-19-space-system-refactor-design.md

ALTER TABLE space_types ADD COLUMN IF NOT EXISTS category text;

-- Seed the new category system types. Use the generic space_fields group (043)
-- for all category parents; existing university sub-type keeps the university
-- group (044).
INSERT INTO space_types (id, tenant_id, name, slug, description, icon, field_group_id, is_system, category) VALUES
    ('00000000-0000-0000-0000-000000000066', '00000000-0000-0000-0000-000000000001', 'Education',   'education',   'Education-related spaces',       'graduation-cap', '00000000-0000-0000-0000-000000000043', true, 'education'),
    ('00000000-0000-0000-0000-000000000067', '00000000-0000-0000-0000-000000000001', 'Business',    'business',    'Business-related spaces',        'briefcase',      '00000000-0000-0000-0000-000000000043', true, 'business'),
    ('00000000-0000-0000-0000-000000000068', '00000000-0000-0000-0000-000000000001', 'Profession',  'profession',  'Profession-based spaces',        'user-check',     '00000000-0000-0000-0000-000000000043', true, 'profession'),
    ('00000000-0000-0000-0000-000000000069', '00000000-0000-0000-0000-000000000001', 'Interest',    'interest',    'Interest-based spaces',          'star',           '00000000-0000-0000-0000-000000000043', true, 'interest'),
    ('00000000-0000-0000-0000-000000000070', '00000000-0000-0000-0000-000000000001', 'Lifestyle',   'lifestyle',   'Lifestyle spaces',               'heart',          '00000000-0000-0000-0000-000000000043', true, 'lifestyle')
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Recategorise existing types.
UPDATE space_types SET category = 'education' WHERE slug = 'university' AND tenant_id = '00000000-0000-0000-0000-000000000001';
UPDATE space_types SET category = 'community'  WHERE slug = 'community'  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- Reassign any existing global spaces to the community type, then delete the obsolete type.
UPDATE spaces
SET space_type_id = (SELECT id FROM space_types WHERE slug = 'community' AND tenant_id = '00000000-0000-0000-0000-000000000001')
WHERE space_type_id = (SELECT id FROM space_types WHERE slug = 'global' AND tenant_id = '00000000-0000-0000-0000-000000000001');

DELETE FROM space_types WHERE slug = 'global' AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- Drop the deprecated is_university flag; category replaces it.
ALTER TABLE space_types DROP COLUMN IF EXISTS is_university;
