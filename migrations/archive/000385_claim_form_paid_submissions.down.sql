-- 000385_claim_form_paid_submissions.down.sql
-- Reverses 000385_claim_form_paid_submissions.up.sql.
-- Drops the seeded claim-ownership form/fields/group, removes the
-- system_settings keys, then drops the three added columns.
--
-- Ref: japaconnect-engineering/sections/06-custom-field-builder-form-builder.md
-- Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md

BEGIN;

-- Remove seeded system_settings.
DELETE FROM system_settings
WHERE key IN ('claim_ownership.form_slug', 'claim_ownership.fee_credits')
  AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- Remove seeded custom fields.
DELETE FROM custom_fields
WHERE id IN (
    '00000000-0000-0000-0000-00000000c011',
    '00000000-0000-0000-0000-00000000c012',
    '00000000-0000-0000-0000-00000000c013',
    '00000000-0000-0000-0000-00000000c014'
);

-- Remove the field-group assignment.
DELETE FROM custom_field_group_assignments
WHERE group_id = '00000000-0000-0000-0000-00000000c001'
  AND entity_class = 'form';

-- Remove the seeded claim-ownership form.
DELETE FROM forms
WHERE id = '00000000-0000-0000-0000-00000000c002';

-- Remove the custom field group.
DELETE FROM custom_field_groups
WHERE id = '00000000-0000-0000-0000-00000000c001';

-- Drop the added columns.
ALTER TABLE ownership_claims DROP COLUMN IF EXISTS form_submission_id;
ALTER TABLE form_submissions DROP COLUMN IF EXISTS payment_id;
ALTER TABLE forms DROP COLUMN IF EXISTS submission_fee_credits;

COMMIT;
