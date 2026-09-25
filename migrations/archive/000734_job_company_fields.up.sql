-- 000734_job_company_fields.up.sql
-- Adds company_logo + company_website to the job_listing_fields group so
-- finder cards can render the recruiting company's logo/name and link out to
-- the employer site, and the composer can upload a logo.
--
-- Ref: japaconnect-engineering/sections/63-jobs-board.md
--      japaconnect-engineering/sections/06-custom-fields-forms.md

BEGIN;

INSERT INTO custom_fields (id, group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config) VALUES
    ('00000000-0000-0000-0000-0000000000b1',
     '00000000-0000-0000-0000-0000000000a0',
     'Company Logo', 'company_logo', 'url', NULL,
     false, true, true, false, false, false, 16, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000b2',
     '00000000-0000-0000-0000-0000000000a0',
     'Company Website', 'company_website', 'url', NULL,
     false, true, true, false, false, false, 17, '{}'::jsonb)
ON CONFLICT (group_id, key) DO NOTHING;

COMMIT;
