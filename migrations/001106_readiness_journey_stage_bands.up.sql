-- 001106_readiness_journey_stage_bands.up.sql
-- Turn journey_stage into a real follow-up driver (§71, ADR-137). The stage
-- options determine the answers to whole field clusters, so asking those
-- fields at the determined extremes is redundant — and each hidden field
-- leaves both scoring numerator and denominator.
--
-- Three visibility bands plus a tightened visa_stage gate:
--
--   visa_stage — applied -> applying and approved -> approved are determined,
--     so it now shows only while journey_stage is unanswered or in
--     {planning, preparing} (considering/relocated/settled were already
--     hidden by migration 001104).
--
--   Band A — visa-process items, determined once the visa is approved
--     (approved/relocated/settled hidden): proof_of_funds, documents_ready,
--     language_test, visa_medical, police_clearance. Still live during
--     `applied` — that is exactly what the user is working through.
--
--   Band B — pre-move execution, determined once moved (relocated/settled
--     hidden): travel_logistics, exit_plan, belongings_shipping,
--     settlement_plan.
--
--   Band C — settling-in tasks, still open while `relocated` but done by
--     `settled`: accommodation, banking_setup, health_insurance,
--     driving_transport, healthcare_continuity, destination_research.
--
-- `considering` keeps all bands shown — not_started answers there are honest
-- low-score signal, not redundancy. credential_evaluation stays ungated: it
-- covers recognition beyond the visa (licensing bodies), so visa approval
-- does not determine it.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

UPDATE custom_fields
SET config = jsonb_set(config, '{visible_if}',
  '{"any":[{"field":"journey_stage","op":"not_answered"},{"field":"journey_stage","op":"in","value":["planning","preparing"]}]}'::jsonb)
WHERE id = '00000000-0000-0000-0000-00000000a021'; -- visa_stage

UPDATE custom_fields
SET config = config ||
  '{"visible_if":{"any":[{"field":"journey_stage","op":"not_answered"},{"field":"journey_stage","op":"in","value":["considering","planning","preparing","applied"]}]}}'::jsonb
WHERE id IN (
  '00000000-0000-0000-0000-00000000c001', -- proof_of_funds
  '00000000-0000-0000-0000-00000000c003', -- documents_ready
  '00000000-0000-0000-0000-00000000d011', -- language_test
  '00000000-0000-0000-0000-00000000e017', -- visa_medical
  '00000000-0000-0000-0000-00000000e018'  -- police_clearance
);

UPDATE custom_fields
SET config = config ||
  '{"visible_if":{"any":[{"field":"journey_stage","op":"not_answered"},{"field":"journey_stage","op":"in","value":["considering","planning","preparing","applied","approved"]}]}}'::jsonb
WHERE id IN (
  '00000000-0000-0000-0000-00000000c005', -- travel_logistics
  '00000000-0000-0000-0000-00000000e019', -- exit_plan
  '00000000-0000-0000-0000-00000000f023', -- belongings_shipping
  '00000000-0000-0000-0000-00000000d013'  -- settlement_plan
);

UPDATE custom_fields
SET config = config ||
  '{"visible_if":{"any":[{"field":"journey_stage","op":"not_answered"},{"field":"journey_stage","op":"in","value":["considering","planning","preparing","applied","approved","relocated"]}]}}'::jsonb
WHERE id IN (
  '00000000-0000-0000-0000-00000000a022', -- accommodation
  '00000000-0000-0000-0000-00000000e020', -- banking_setup
  '00000000-0000-0000-0000-00000000c004', -- health_insurance
  '00000000-0000-0000-0000-00000000f026', -- driving_transport
  '00000000-0000-0000-0000-00000000f025', -- healthcare_continuity
  '00000000-0000-0000-0000-00000000d012'  -- destination_research
);

COMMIT;
