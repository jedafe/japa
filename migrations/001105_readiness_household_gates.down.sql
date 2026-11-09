-- 001105_readiness_household_gates.down.sql
-- Reverse the household gates and remove the moving_with driver field.
-- Stored moving_with values in custom_field_values are deleted first to
-- satisfy the field_id foreign key.

BEGIN;

UPDATE custom_fields
SET config = config - 'visible_if'
WHERE id IN (
  '00000000-0000-0000-0000-00000000c009', -- dependents_plan
  '00000000-0000-0000-0000-00000000f024', -- pets_relocation
  '00000000-0000-0000-0000-00000000f029'  -- spouse_career
);

DELETE FROM custom_field_values
WHERE field_id = '00000000-0000-0000-0000-00000000b007';

DELETE FROM custom_fields
WHERE id = '00000000-0000-0000-0000-00000000b007'; -- moving_with

COMMIT;
