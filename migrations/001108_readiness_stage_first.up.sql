-- 001108_readiness_stage_first.up.sql
-- Make journey_stage the first readiness question and hide purpose for
-- post-move users (§71, ADR-137).
--
-- "Where are you in your relocation journey?" is the primary driver of the
-- conditional questionnaire — its options shed follow-ups via the 001106/001107
-- journey bands — so it is asked before the other context fields.
-- destination_country and origin_country shift to positions 2 and 3.
--
-- "Why are you relocating?" is meaningless once a user has selected
-- relocated/settled — the move already happened. It gets the same Band B
-- visible_if used by the pre-move execution fields (001106): shown while
-- journey_stage is unanswered or any pre-move stage.
--
-- Post-move users keep a purpose signal for roadmap matching through
-- visa_type → PurposeForVisaRoute backfill in parseJourneyFields and the
-- visitor submission overlay: their explicit route supplies the purpose.
-- The dependent gates degrade gracefully — visa_type shows on
-- purpose-not_answered and supplies the route the study/business gates read.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

-- Rotate the first three context fields: journey_stage first.
UPDATE custom_fields SET position = 1
WHERE group_id = '00000000-0000-0000-0000-00000000a001' AND key = 'journey_stage';
UPDATE custom_fields SET position = 2
WHERE group_id = '00000000-0000-0000-0000-00000000a001' AND key = 'destination_country';
UPDATE custom_fields SET position = 3
WHERE group_id = '00000000-0000-0000-0000-00000000a001' AND key = 'origin_country';

-- Hide purpose at relocated/settled (Band B shape, fail-open on unanswered).
UPDATE custom_fields
SET config = config ||
  '{"visible_if":{"any":[{"field":"journey_stage","op":"not_answered"},{"field":"journey_stage","op":"in","value":["considering","planning","preparing","applied","approved"]}]}}'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000b005'; -- purpose

COMMIT;
