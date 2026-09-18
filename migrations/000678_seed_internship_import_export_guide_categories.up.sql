-- 000678_seed_internship_import_export_guide_categories.up.sql
-- Section 14 (Content Type System) — Seed the "Internship" and
-- "Import and Export" guide categories, their custom field groups,
-- and a starter set of custom fields for each.
--
-- Each category gets:
--   1. a custom_field_entity_types row (value = 'guide_category:{slug}')
--   2. a custom_field_groups row (slug = 'guide_{slug}_fields')
--   3. a custom_field_group_assignments row
--      (entity_class = 'content_type:guide:{slug}')
--   4. starter custom_fields rows
--
-- Ref: japaconnect-engineering/sections/14-content-type-system.md
-- Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md

BEGIN;

-- ============================================================
-- 1. Guide categories
-- ============================================================
INSERT INTO custom_field_entity_types (value, label, is_system, sort_order)
VALUES
    ('guide_category:internship',    'Internship',        false, 11),
    ('guide_category:import_export', 'Import & Export',   false, 12)
ON CONFLICT (value) DO NOTHING;

-- ============================================================
-- 2. Field groups
-- ============================================================
INSERT INTO custom_field_groups (name, slug, description, is_system)
VALUES
    ('Internship Fields',      'guide_internship_fields',    'Custom fields for internship guide content.',      false),
    ('Import & Export Fields', 'guide_import_export_fields', 'Custom fields for import and export guide content.', false)
ON CONFLICT (slug) DO NOTHING;

-- ============================================================
-- 3. Group assignments to guide category entity classes
--    (entity_id is NULL — NOT EXISTS used because NULL does not
--    participate in unique-conflict detection)
-- ============================================================
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'content_type:guide:internship', NULL, 0
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = g.id AND a.entity_class = 'content_type:guide:internship'
  );

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'content_type:guide:import_export', NULL, 0
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = g.id AND a.entity_class = 'content_type:guide:import_export'
  );

-- ============================================================
-- 4. Internship fields
-- ============================================================
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Destination Country', 'destination_country', 'suggest', '{"source":"geo_countries"}'::jsonb, true, false, true, true, false, false, 1
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Host Organization', 'host_organization', 'text', NULL, false, false, true, true, true, true, 2
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Internship Type', 'internship_type', 'select', '[{"value":"summer","label":"Summer Internship"},{"value":"industrial_attachment","label":"Industrial Attachment"},{"value":"graduate","label":"Graduate Internship"},{"value":"research","label":"Research Internship"},{"value":"remote","label":"Remote Internship"},{"value":"coop","label":"Co-op Program"}]'::jsonb, true, false, true, true, false, false, 3
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Industry', 'industry', 'select', '[{"value":"technology","label":"Technology"},{"value":"finance","label":"Finance"},{"value":"engineering","label":"Engineering"},{"value":"healthcare","label":"Healthcare"},{"value":"media","label":"Media & Communications"},{"value":"law","label":"Law"},{"value":"ngo","label":"NGO & Development"},{"value":"other","label":"Other"}]'::jsonb, false, false, true, true, false, false, 4
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Duration', 'duration', 'text', NULL, false, false, true, true, true, true, 5
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Paid Internship', 'is_paid', 'boolean', NULL, false, false, true, true, false, false, 6
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Monthly Stipend (USD)', 'monthly_stipend_usd', 'number', NULL, false, false, true, true, true, false, 7
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Visa Sponsorship Available', 'visa_sponsorship', 'boolean', NULL, false, false, true, true, false, false, 8
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Eligibility Criteria', 'eligibility_criteria', 'textarea', NULL, true, false, true, false, false, true, 9
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Application Deadline', 'application_deadline', 'date', NULL, false, false, true, true, true, false, 10
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Application Link', 'application_link', 'url', NULL, false, false, true, false, false, false, 11
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Required Documents', 'required_documents', 'textarea', NULL, false, false, true, false, false, true, 12
FROM custom_field_groups g
WHERE g.slug = 'guide_internship_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- ============================================================
-- 5. Import and Export fields
-- ============================================================
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Origin Country', 'origin_country', 'suggest', '{"source":"geo_countries"}'::jsonb, true, false, true, true, false, false, 1
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Destination Country', 'destination_country', 'suggest', '{"source":"geo_countries"}'::jsonb, true, false, true, true, false, false, 2
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Product Category', 'product_category', 'select', '[{"value":"agriculture_food","label":"Agriculture & Food"},{"value":"textiles_apparel","label":"Textiles & Apparel"},{"value":"electronics","label":"Electronics"},{"value":"machinery","label":"Machinery & Equipment"},{"value":"cosmetics","label":"Cosmetics & Beauty"},{"value":"arts_crafts","label":"Arts & Crafts"},{"value":"other","label":"Other"}]'::jsonb, true, false, true, true, false, false, 3
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'HS Code', 'hs_code', 'text', NULL, false, false, true, true, true, true, 4
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Trade Direction', 'trade_direction', 'select', '[{"value":"import","label":"Import"},{"value":"export","label":"Export"},{"value":"both","label":"Both"}]'::jsonb, false, false, true, true, false, false, 5
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Incoterms', 'incoterms', 'select', '[{"value":"exw","label":"EXW — Ex Works"},{"value":"fca","label":"FCA — Free Carrier"},{"value":"fob","label":"FOB — Free On Board"},{"value":"cfr","label":"CFR — Cost & Freight"},{"value":"cif","label":"CIF — Cost, Insurance & Freight"},{"value":"dap","label":"DAP — Delivered At Place"},{"value":"ddp","label":"DDP — Delivered Duty Paid"}]'::jsonb, false, false, true, true, false, false, 6
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Shipping Method', 'shipping_method', 'select', '[{"value":"sea","label":"Sea Freight"},{"value":"air","label":"Air Freight"},{"value":"land","label":"Land Transport"},{"value":"courier","label":"Courier / Express"}]'::jsonb, false, false, true, true, false, false, 7
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Estimated Timeline', 'estimated_timeline', 'text', NULL, false, false, true, true, true, true, 8
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Customs Duty Rate (%)', 'customs_duty_rate_pct', 'number', NULL, false, false, true, true, true, false, 9
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Regulatory Agency', 'regulatory_agency', 'text', NULL, false, false, true, true, true, true, 10
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Required Licenses & Permits', 'required_licenses_permits', 'textarea', NULL, false, false, true, false, false, true, 11
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Required Documents', 'required_documents', 'textarea', NULL, false, false, true, false, false, true, 12
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Official Portal Link', 'official_portal_link', 'url', NULL, false, false, true, false, false, false, 13
FROM custom_field_groups g
WHERE g.slug = 'guide_import_export_fields'
ON CONFLICT (group_id, key) DO NOTHING;

COMMIT;
