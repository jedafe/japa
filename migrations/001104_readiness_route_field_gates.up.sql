-- 001104_readiness_route_field_gates.up.sql
-- Gate route/applicability-specific readiness fields on their driver answers
-- (§71, ADR-137). Fields whose applicability is already determined by an
-- earlier answer are hidden and excluded from both scoring numerator and
-- denominator — removing the 90-point "not applicable" dilution that
-- non-applicable users currently absorb.
--
-- Gates applied:
--   study_funding, admission_status — only for the study route
--     (purpose=study OR visa_type=student)
--   business_readiness — only for the business route
--     (purpose=business OR visa_type=business)
--   visa_stage — hidden at the journey extremes where visa state is
--     already determined (considering -> not started; relocated/settled ->
--     approved); shown while the stage is unanswered or ambiguous
--
-- Also fixes the savings label: its options are durations of expenses, not
-- amounts.
--
-- All updates are idempotent via deterministic UUIDs and jsonb merge.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

UPDATE custom_fields
SET config = config || '{"visible_if":{"any":[{"field":"purpose","op":"eq","value":"study"},{"field":"visa_type","op":"eq","value":"student"}]}}'::jsonb
WHERE id IN (
  '00000000-0000-0000-0000-00000000e014', -- study_funding
  '00000000-0000-0000-0000-00000000e016'  -- admission_status
);

UPDATE custom_fields
SET config = config || '{"visible_if":{"any":[{"field":"purpose","op":"eq","value":"business"},{"field":"visa_type","op":"eq","value":"business"}]}}'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f027'; -- business_readiness

UPDATE custom_fields
SET config = config || '{"visible_if":{"any":[{"field":"journey_stage","op":"not_answered"},{"field":"journey_stage","op":"in","value":["planning","preparing","applied","approved"]}]}}'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000a021'; -- visa_stage

UPDATE custom_fields
SET label = 'How many months of living expenses have you saved for your move?'
WHERE id = '00000000-0000-0000-0000-00000000a011'; -- savings

COMMIT;
