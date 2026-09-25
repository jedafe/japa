-- BUG-004: Seed field definitions for the Feedback and Report system forms.
-- The forms were seeded in migration 000008 but had no field groups, no
-- assignments, and no fields — so the /feedback page rendered an empty form.
-- Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
--   feedback_form → Category, Description, Screenshot
--   report_form   → Reason, Evidence, Additional Notes

-- Feedback form field group (ID ...050)
INSERT INTO custom_field_groups (id, tenant_id, name, slug, is_system)
VALUES ('00000000-0000-0000-0000-000000000050', '00000000-0000-0000-0000-000000000001', 'Feedback Form Fields', 'form-feedback-fields', true)
ON CONFLICT (id) DO NOTHING;

-- Report form field group (ID ...051)
INSERT INTO custom_field_groups (id, tenant_id, name, slug, is_system)
VALUES ('00000000-0000-0000-0000-000000000051', '00000000-0000-0000-0000-000000000001', 'Report Form Fields', 'form-report-fields', true)
ON CONFLICT (id) DO NOTHING;

-- Assign the groups to the forms (entity_class='form', entity_id=form_id)
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES
    ('00000000-0000-0000-0000-000000000050', 'form', '00000000-0000-0000-0000-000000000047', 0),
    ('00000000-0000-0000-0000-000000000051', 'form', '00000000-0000-0000-0000-000000000048', 0)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

-- Feedback form fields: Category (select), Description (textarea), Screenshot (image_single)
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, is_system, is_required, position) VALUES
    ('00000000-0000-0000-0000-000000000052', '00000000-0000-0000-0000-000000000050', '00000000-0000-0000-0000-000000000001',
     'Category', 'category', 'select',
     '[{"value":"bug","label":"Bug Report"},{"value":"feature","label":"Feature Request"},{"value":"question","label":"Question"},{"value":"other","label":"Other"}]'::jsonb,
     true, true, 0),
    ('00000000-0000-0000-0000-000000000053', '00000000-0000-0000-0000-000000000050', '00000000-0000-0000-0000-000000000001',
     'Description', 'description', 'textarea', NULL, true, true, 1),
    ('00000000-0000-0000-0000-000000000054', '00000000-0000-0000-0000-000000000050', '00000000-0000-0000-0000-000000000001',
     'Screenshot', 'screenshot', 'image_single', NULL, true, false, 2)
ON CONFLICT (id) DO NOTHING;

-- Report form fields: Reason (select), Evidence (textarea), Additional Notes (textarea)
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, is_system, is_required, position) VALUES
    ('00000000-0000-0000-0000-000000000055', '00000000-0000-0000-0000-000000000051', '00000000-0000-0000-0000-000000000001',
     'Reason', 'reason', 'select',
     '[{"value":"spam","label":"Spam"},{"value":"harassment","label":"Harassment"},{"value":"misinformation","label":"Misinformation"},{"value":"illegal","label":"Illegal Content"},{"value":"other","label":"Other"}]'::jsonb,
     true, true, 0),
    ('00000000-0000-0000-0000-000000000056', '00000000-0000-0000-0000-000000000051', '00000000-0000-0000-0000-000000000001',
     'Evidence', 'evidence', 'textarea', NULL, true, true, 1),
    ('00000000-0000-0000-0000-000000000057', '00000000-0000-0000-0000-000000000051', '00000000-0000-0000-0000-000000000001',
     'Additional Notes', 'additional_notes', 'textarea', NULL, true, false, 2)
ON CONFLICT (id) DO NOTHING;
