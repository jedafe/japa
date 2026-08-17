-- 000509_seed_visa_guide_request_form.down.sql
-- Reverses 000509_seed_visa_guide_request_form.up.sql.
-- Ref: japaconnect-engineering/sections/70-visa-checklist.md

BEGIN;

-- Delete the form's fields (cascade from group would also handle this,
-- but explicit deletes make the down migration self-documenting).
DELETE FROM custom_fields
WHERE id IN (
    '00000000-0000-0000-0000-000000000107',
    '00000000-0000-0000-0000-000000000108',
    '00000000-0000-0000-0000-000000000109',
    '00000000-0000-0000-0000-000000000110'
);

-- Delete the field-group assignment.
DELETE FROM custom_field_group_assignments
WHERE group_id = '00000000-0000-0000-0000-000000000106'
  AND entity_class = 'form'
  AND entity_id = '00000000-0000-0000-0000-000000000105';

-- Delete the field group.
DELETE FROM custom_field_groups
WHERE id = '00000000-0000-0000-0000-000000000106';

-- Delete any submissions to the form (so the form row can be removed
-- without FK violations on form_submissions.form_id).
DELETE FROM form_submissions
WHERE form_id = '00000000-0000-0000-0000-000000000105';

-- Delete the form.
DELETE FROM forms
WHERE id = '00000000-0000-0000-0000-000000000105';

COMMIT;
