-- 000475_seed_jobs_board_fields.up.sql
-- Jobs Board field group + custom fields + application form (Section 63, ADR-081).
-- The Jobs Board stores job-specific attributes in custom_field_values under
-- the job_listing_fields group. Without this seed the finder filters, the
-- is_featured toggle, the application_deadline expiry task, and the typed
-- value columns (value_bool / value_number) all fail or no-op because there
-- are no custom_fields rows to resolve field_id from.
--
-- Also seeds the job_application_form system form (ADR-081 §2) so the
-- on-platform apply flow has a canonical form template.
--
-- Ref: japaconnect-engineering/sections/63-jobs-board.md
--      japaconnect-engineering/adr/ADR-081-jobs-board.md
--      japaconnect-engineering/sections/06-custom-fields-forms.md

BEGIN;

-- =====================================================================
-- job_listing_fields custom field group (entity_class = 'content')
-- =====================================================================
INSERT INTO custom_field_groups (id, tenant_id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-0000000000a0',
    '00000000-0000-0000-0000-000000000001',
    'Job Listing Fields',
    'job_listing_fields',
    'Job-specific attributes for job_listing content (Section 63, ADR-081).',
    true
)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Attach the group to the content entity class (job_listing discriminator).
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES ('00000000-0000-0000-0000-0000000000a0', 'content', NULL, 0)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

-- =====================================================================
-- 15 job listing custom fields.
-- field_type drives which value_* column the typed upsert helper writes to:
--   text/textarea/url/select -> value_text
--   number                   -> value_number
--   boolean                  -> value_bool
--   date                     -> value_text (ISO date stored as text; the
--                               expiry task casts value_text::timestamptz)
-- =====================================================================
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config) VALUES
    ('00000000-0000-0000-0000-0000000000a1',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Company Name', 'company_name', 'text', NULL,
     true, true, true, false, false, true, 1, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000a2',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Job Type', 'job_type', 'select',
     '[{"value":"full_time","label":"Full-time"},{"value":"part_time","label":"Part-time"},{"value":"contract","label":"Contract"},{"value":"freelance","label":"Freelance"},{"value":"internship","label":"Internship"}]'::jsonb,
     true, true, true, true, false, true, 2, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000a3',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Location Type', 'location_type', 'select',
     '[{"value":"onsite","label":"On-site"},{"value":"remote","label":"Remote"},{"value":"hybrid","label":"Hybrid"}]'::jsonb,
     true, true, true, true, false, true, 3, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000a4',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Country', 'country', 'text', NULL,
     false, true, true, true, false, true, 4, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000a5',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'City', 'city', 'text', NULL,
     false, true, true, false, false, true, 5, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000a6',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Salary Min', 'salary_min', 'number', NULL,
     false, true, true, false, true, true, 6, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000a7',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Salary Max', 'salary_max', 'number', NULL,
     false, true, true, false, true, true, 7, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000a8',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Salary Currency', 'salary_currency', 'text', NULL,
     false, true, true, false, false, true, 8, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000a9',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Experience Level', 'experience_level', 'select',
     '[{"value":"entry","label":"Entry"},{"value":"mid","label":"Mid"},{"value":"senior","label":"Senior"},{"value":"lead","label":"Lead"},{"value":"executive","label":"Executive"}]'::jsonb,
     false, true, true, true, false, true, 9, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000aa',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Skills Required', 'skills_required', 'textarea', NULL,
     false, true, true, false, false, true, 10, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000ab',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Application URL', 'application_url', 'url', NULL,
     false, true, true, false, false, true, 11, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000ac',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Application Deadline', 'application_deadline', 'date', NULL,
     false, true, true, false, true, true, 12, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000ad',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Visa Sponsorship', 'visa_sponsorship', 'boolean', NULL,
     false, true, true, true, false, true, 13, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000ae',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Relocation Assistance', 'relocation_assistance', 'boolean', NULL,
     false, true, true, false, false, true, 14, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000af',
     '00000000-0000-0000-0000-0000000000a0', '00000000-0000-0000-0000-000000000001',
     'Featured', 'is_featured', 'boolean', NULL,
     false, true, false, false, true, false, 15, '{}'::jsonb)
ON CONFLICT (group_id, key) DO NOTHING;

-- =====================================================================
-- job_application_form — system form template for on-platform applications
-- (ADR-081 §2). Employers/admins extend it with role-specific fields via
-- the Form Builder; the system row is the minimum canonical template.
-- =====================================================================
INSERT INTO forms (id, tenant_id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-0000000000b0',
    '00000000-0000-0000-0000-000000000001',
    'Job Application Form',
    'job_application_form',
    'Default on-platform job application form (Section 63, ADR-081).',
    true
)
ON CONFLICT (tenant_id, slug) DO NOTHING;

COMMIT;
