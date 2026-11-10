-- 001106_readiness_journey_stage_bands.down.sql
-- Reverse the journey-stage bands: restore visa_stage's 001104 gate and
-- remove visible_if from the band A/B/C fields.

BEGIN;

UPDATE custom_fields
SET config = jsonb_set(config, '{visible_if}',
  '{"any":[{"field":"journey_stage","op":"not_answered"},{"field":"journey_stage","op":"in","value":["planning","preparing","applied","approved"]}]}'::jsonb)
WHERE id = '00000000-0000-0000-0000-00000000a021'; -- visa_stage

UPDATE custom_fields
SET config = config - 'visible_if'
WHERE id IN (
  '00000000-0000-0000-0000-00000000c001', -- proof_of_funds
  '00000000-0000-0000-0000-00000000c003', -- documents_ready
  '00000000-0000-0000-0000-00000000d011', -- language_test
  '00000000-0000-0000-0000-00000000e017', -- visa_medical
  '00000000-0000-0000-0000-00000000e018', -- police_clearance
  '00000000-0000-0000-0000-00000000c005', -- travel_logistics
  '00000000-0000-0000-0000-00000000e019', -- exit_plan
  '00000000-0000-0000-0000-00000000f023', -- belongings_shipping
  '00000000-0000-0000-0000-00000000d013', -- settlement_plan
  '00000000-0000-0000-0000-00000000a022', -- accommodation
  '00000000-0000-0000-0000-00000000e020', -- banking_setup
  '00000000-0000-0000-0000-00000000c004', -- health_insurance
  '00000000-0000-0000-0000-00000000f026', -- driving_transport
  '00000000-0000-0000-0000-00000000f025', -- healthcare_continuity
  '00000000-0000-0000-0000-00000000d012'  -- destination_research
);

COMMIT;
