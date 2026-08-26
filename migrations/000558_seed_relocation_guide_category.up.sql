-- 000558_seed_relocation_guide_category.up.sql
-- Section 14 (Content Type System) — Seed the relocation guide category,
-- its custom field group, and a starter set of custom fields.
--
-- This makes the "20 relocation-to-Canada guides for West African countries"
-- scenario actually runnable: the LLM can discover the category via
-- guide_categories.list, fetch its fields via custom_fields.list, and then
-- populate structured custom field values for each generated guide.
--
-- Ref: japaconnect-engineering/sections/14-content-type-system.md
-- Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md

BEGIN;

-- 1. Ensure the relocation guide category exists.
INSERT INTO custom_field_entity_types (tenant_id, value, label, is_system)
VALUES ('00000000-0000-0000-0000-000000000001', 'guide_category:relocation', 'Relocation Guide', false)
ON CONFLICT (tenant_id, value) DO NOTHING;

-- 2. Ensure the relocation field group exists.
INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
VALUES ('00000000-0000-0000-0000-000000000001', 'Relocation Guide Fields', 'guide_relocation_fields', 'Custom fields for relocation guides.', false)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- 3. Assign the group to the guide:relocation entity class (idempotent).
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'content_type:guide:relocation', NULL, 0
FROM custom_field_groups g
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND g.slug = 'guide_relocation_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = g.id AND a.entity_class = 'content_type:guide:relocation'
  );

-- 4. Seed starter fields for the relocation guide category (idempotent by group_id + key).
INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, '00000000-0000-0000-0000-000000000001', 'Origin Country', 'origin_country', 'text', NULL, true, false, true, true, true, true, 1
FROM custom_field_groups g
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001' AND g.slug = 'guide_relocation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, '00000000-0000-0000-0000-000000000001', 'Destination Country', 'destination_country', 'text', NULL, true, false, true, true, true, true, 2
FROM custom_field_groups g
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001' AND g.slug = 'guide_relocation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, '00000000-0000-0000-0000-000000000001', 'Relocation Purpose', 'relocation_purpose', 'select', '[{"value":"work","label":"Work"},{"value":"study","label":"Study"},{"value":"family","label":"Family"},{"value":"business","label":"Business"},{"value":"retirement","label":"Retirement"}]'::jsonb, false, false, true, true, false, false, 3
FROM custom_field_groups g
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001' AND g.slug = 'guide_relocation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, '00000000-0000-0000-0000-000000000001', 'Key Steps', 'key_steps', 'textarea', NULL, false, false, true, false, false, true, 4
FROM custom_field_groups g
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001' AND g.slug = 'guide_relocation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, '00000000-0000-0000-0000-000000000001', 'Estimated Cost (USD)', 'estimated_cost_usd', 'number', NULL, false, false, true, true, true, false, 5
FROM custom_field_groups g
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001' AND g.slug = 'guide_relocation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, '00000000-0000-0000-0000-000000000001', 'Timeline (Months)', 'timeline_months', 'number', NULL, false, false, true, true, true, false, 6
FROM custom_field_groups g
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001' AND g.slug = 'guide_relocation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, '00000000-0000-0000-0000-000000000001', 'Official Resources', 'official_resources', 'textarea', NULL, false, false, true, false, false, false, 7
FROM custom_field_groups g
WHERE g.tenant_id = '00000000-0000-0000-0000-000000000001' AND g.slug = 'guide_relocation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

COMMIT;
