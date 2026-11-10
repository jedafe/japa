-- 001120_seed_dataset_guide_category.up.sql
-- Section 14 (Content Type System) — Seed the "Dataset" guide category.
-- The intelligence promoter maps entity_type='dataset' records to
-- contents rows with content_type='guide' + guide_category='dataset'
-- (internal/server promoter registry), but the category entity type was
-- never seeded — before 001117's contents.guide_category trigger this
-- silently minted a phantom category; after it, the insert would be
-- rejected. Seeds follow the 000678 internship/import_export pattern:
-- entity type + field group + assignment + starter fields matching the
-- extraction schema keys (title, description, category, value, unit,
-- country, effective_date, data, source_url). All fields optional —
-- dataset guides are machine-authored and sparse field_values must not
-- fail required-field validation on unrelated edits.
--
-- Ref: japaconnect-engineering/sections/14-content-type-system.md

BEGIN;

-- 1. Guide category
INSERT INTO custom_field_entity_types (value, label, is_system, sort_order)
VALUES ('guide_category:dataset', 'Dataset', false, 13)
ON CONFLICT (value) DO NOTHING;

-- 2. Field group
INSERT INTO custom_field_groups (name, slug, description, is_system)
VALUES ('Dataset Fields', 'guide_dataset_fields', 'Custom fields for dataset guide content.', false)
ON CONFLICT (slug) DO NOTHING;

-- 3. Group assignment (entity_id is NULL — NOT EXISTS used because NULL
--    does not participate in unique-conflict detection)
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'content_type:guide:dataset', NULL, 0
FROM custom_field_groups g
WHERE g.slug = 'guide_dataset_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = g.id AND a.entity_class = 'content_type:guide:dataset'
  );

-- 4. Dataset fields — keys match the intelligence extraction schema.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Description', 'description', 'textarea', NULL, false, false, true, false, false, true, 1
FROM custom_field_groups g WHERE g.slug = 'guide_dataset_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Category', 'category', 'text', NULL, false, false, true, true, false, false, 2
FROM custom_field_groups g WHERE g.slug = 'guide_dataset_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Value', 'value', 'text', NULL, false, false, true, true, true, true, 3
FROM custom_field_groups g WHERE g.slug = 'guide_dataset_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Unit', 'unit', 'text', NULL, false, false, true, true, false, false, 4
FROM custom_field_groups g WHERE g.slug = 'guide_dataset_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Country', 'country', 'suggest', '{"source":"geo_countries"}'::jsonb, false, false, true, true, false, true, 5
FROM custom_field_groups g WHERE g.slug = 'guide_dataset_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Effective Date', 'effective_date', 'date', NULL, false, false, true, true, true, false, 6
FROM custom_field_groups g WHERE g.slug = 'guide_dataset_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Data', 'data', 'textarea', NULL, false, false, true, false, false, false, 7
FROM custom_field_groups g WHERE g.slug = 'guide_dataset_fields'
ON CONFLICT (group_id, key) DO NOTHING;

COMMIT;
