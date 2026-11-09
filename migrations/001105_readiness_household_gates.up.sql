-- 001105_readiness_household_gates.up.sql
-- Add the `moving_with` household-profile driver (unscored multiselect) and
-- gate the three companion-specific scored fields on it (§71, ADR-137).
-- Until now each of these fields asked every user to self-declare
-- applicability via a 90-point "not applicable" escape option, diluting the
-- dimension denominator for users the question never applied to.
--
-- Gates applied:
--   dependents_plan — shown when moving_with is unanswered or contains
--     "dependents"
--   spouse_career — shown when moving_with is unanswered or contains
--     "partner"
--   pets_relocation — shown when moving_with is unanswered or contains
--     "pets"
--
-- driving_transport keeps its not_driving escape: intent to drive at the
-- destination is not derivable from any existing answer, and adding a
-- separate would-drive driver question to remove one escape is net-zero.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

INSERT INTO custom_fields
  (id, group_id, label, key, field_type, options, placeholder,
   is_required, is_system, show_in_frontend, position, config)
VALUES (
  '00000000-0000-0000-0000-00000000b007',
  '00000000-0000-0000-0000-00000000a001',
  'Who is relocating with you? Select all that apply.',
  'moving_with', 'multiselect',
  '[{"value":"alone","label":"Just me — moving alone"},{"value":"partner","label":"Spouse or partner"},{"value":"dependents","label":"Children or other dependants"},{"value":"pets","label":"Pets"}]'::jsonb,
  'Select all that apply', false, false, true, 7,
  '{"weight":0,"dimension":"","option_scores":{}}'::jsonb
)
ON CONFLICT (id) DO NOTHING;

UPDATE custom_fields
SET config = config || '{"visible_if":{"any":[{"field":"moving_with","op":"not_answered"},{"field":"moving_with","op":"contains","value":"dependents"}]}}'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000c009'; -- dependents_plan

UPDATE custom_fields
SET config = config || '{"visible_if":{"any":[{"field":"moving_with","op":"not_answered"},{"field":"moving_with","op":"contains","value":"pets"}]}}'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f024'; -- pets_relocation

UPDATE custom_fields
SET config = config || '{"visible_if":{"any":[{"field":"moving_with","op":"not_answered"},{"field":"moving_with","op":"contains","value":"partner"}]}}'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f029'; -- spouse_career

COMMIT;
