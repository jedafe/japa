-- 001104_readiness_route_field_gates.down.sql
-- Reverse the route/applicability gates and restore the savings label.

BEGIN;

UPDATE custom_fields
SET config = config - 'visible_if'
WHERE id IN (
  '00000000-0000-0000-0000-00000000e014', -- study_funding
  '00000000-0000-0000-0000-00000000e016', -- admission_status
  '00000000-0000-0000-0000-00000000f027', -- business_readiness
  '00000000-0000-0000-0000-00000000a021'  -- visa_stage
);

UPDATE custom_fields
SET label = 'How much have you saved for your move?'
WHERE id = '00000000-0000-0000-0000-00000000a011'; -- savings

COMMIT;
