-- 001107_readiness_postmove_fixes.up.sql
-- Post-move follow-up fixes for the readiness questionnaire (§71, ADR-137).
-- After migration 001106's journey bands, relocated/settled users still saw
-- questions whose wording or purpose only makes sense pre-move.
--
-- Hides (gates):
--   travel_date — "planning to travel or move" is retrospective post-move
--   relocation_budget — the move's budget is already spent
--   emergency_buffer — "first months" is live while relocated, past when
--     settled (Band C)
--
-- Label rewords (tense-neutral so the same question reads correctly at
-- every journey_stage):
--   destination_country / origin_country — "relocating to/from" works for
--     both pre- and post-move users. This also fixes a data bug: the old
--     origin_country label ("Which country are you currently in?") made
--     relocated users store their destination as JourneyContext.OriginCountry.
--   visa_type, income_source, savings, emotional_readiness,
--   health_insurance, banking_setup, emergency_buffer, moving_with —
--   same neutral treatment ("arrival"/"pursuing"/"the move" -> destination/
--   applies/removed tense).
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

-- Gates ------------------------------------------------------------------

UPDATE custom_fields
SET config = config ||
  '{"visible_if":{"any":[{"field":"journey_stage","op":"not_answered"},{"field":"journey_stage","op":"in","value":["considering","planning","preparing","applied","approved"]}]}}'::jsonb
WHERE id IN (
  '00000000-0000-0000-0000-00000000b006', -- travel_date
  '00000000-0000-0000-0000-00000000c002'  -- relocation_budget
);

UPDATE custom_fields
SET config = config ||
  '{"visible_if":{"any":[{"field":"journey_stage","op":"not_answered"},{"field":"journey_stage","op":"in","value":["considering","planning","preparing","applied","approved","relocated"]}]}}'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000e015'; -- emergency_buffer

-- Tense-neutral labels ----------------------------------------------------

UPDATE custom_fields SET label = 'Which country are you relocating to?'
WHERE id = '00000000-0000-0000-0000-00000000b001'; -- destination_country

UPDATE custom_fields SET label = 'Which country are you relocating from?'
WHERE id = '00000000-0000-0000-0000-00000000b002'; -- origin_country

UPDATE custom_fields SET label = 'Which visa route applies to your move?'
WHERE id = '00000000-0000-0000-0000-00000000b003'; -- visa_type

UPDATE custom_fields SET label = 'Do you have a stable income source at your destination?'
WHERE id = '00000000-0000-0000-0000-00000000a012'; -- income_source

UPDATE custom_fields SET label = 'How many months of living expenses do you currently have saved?'
WHERE id = '00000000-0000-0000-0000-00000000a011'; -- savings

UPDATE custom_fields SET label = 'How are you feeling about the move emotionally?'
WHERE id = '00000000-0000-0000-0000-00000000a041'; -- emotional_readiness

UPDATE custom_fields SET label = 'Is health insurance or healthcare coverage arranged at your destination?'
WHERE id = '00000000-0000-0000-0000-00000000c004'; -- health_insurance

UPDATE custom_fields SET label = 'Have you sorted how you''ll bank and access money at your destination?'
WHERE id = '00000000-0000-0000-0000-00000000e020'; -- banking_setup

UPDATE custom_fields SET label = 'Do you have an emergency buffer for your first months at the destination?'
WHERE id = '00000000-0000-0000-0000-00000000e015'; -- emergency_buffer

UPDATE custom_fields SET label = 'Who is making this move with you? Select all that apply.'
WHERE id = '00000000-0000-0000-0000-00000000b007'; -- moving_with

COMMIT;
