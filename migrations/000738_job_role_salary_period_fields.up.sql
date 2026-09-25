-- 000738_job_role_salary_period_fields.up.sql
-- Adds job_role (canonical role title, distinct from the listing title),
-- job_function (function/department select), and salary_period (pay period
-- select) to the job_listing_fields group so listings can say "Backend
-- Engineer · Engineering" and "USD 5,000–8,000 per month".
--
-- Positions are renumbered so the composer shows a logical order:
-- job_role/job_function slot in after job_type (position 2) and
-- salary_period right after salary_currency.
--
-- Ref: japaconnect-engineering/sections/63-jobs-board.md
--      japaconnect-engineering/sections/06-custom-fields-forms.md

BEGIN;

-- Shift every field at position >= 3 two slots to make room for
-- job_role (3) and job_function (4). After this shift salary_currency
-- sits at 10 and experience_level at 11 — bump position >= 11 once more
-- so salary_period can slot in at 11 between them.
UPDATE custom_fields
SET position = position + 2
WHERE group_id = '00000000-0000-0000-0000-0000000000a0'
  AND position >= 3;

UPDATE custom_fields
SET position = position + 1
WHERE group_id = '00000000-0000-0000-0000-0000000000a0'
  AND position >= 11;

INSERT INTO custom_fields (id, group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position, config) VALUES
    ('00000000-0000-0000-0000-0000000000b3',
     '00000000-0000-0000-0000-0000000000a0',
     'Job Role', 'job_role', 'text', NULL,
     false, true, true, false, false, true, 3, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000b4',
     '00000000-0000-0000-0000-0000000000a0',
     'Job Function', 'job_function', 'select',
     '[{"value":"engineering","label":"Engineering"},{"value":"design","label":"Design"},{"value":"product","label":"Product"},{"value":"data","label":"Data & Analytics"},{"value":"marketing","label":"Marketing"},{"value":"sales","label":"Sales"},{"value":"operations","label":"Operations"},{"value":"finance","label":"Finance"},{"value":"hr","label":"Human Resources"},{"value":"customer_support","label":"Customer Support"},{"value":"legal","label":"Legal"},{"value":"healthcare","label":"Healthcare"},{"value":"education","label":"Education"},{"value":"other","label":"Other"}]'::jsonb,
     false, true, true, true, false, true, 4, '{}'::jsonb),
    ('00000000-0000-0000-0000-0000000000b5',
     '00000000-0000-0000-0000-0000000000a0',
     'Salary Period', 'salary_period', 'select',
     '[{"value":"hourly","label":"Hourly"},{"value":"daily","label":"Daily"},{"value":"weekly","label":"Weekly"},{"value":"biweekly","label":"Bi-weekly"},{"value":"monthly","label":"Monthly"},{"value":"yearly","label":"Yearly"}]'::jsonb,
     false, true, true, false, false, true, 11, '{}'::jsonb)
ON CONFLICT (group_id, key) DO NOTHING;

COMMIT;
