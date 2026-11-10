-- 001109_readiness_stage_neutral_labels.up.sql
-- Choice-aware question wording for the readiness questionnaire (§71,
-- ADR-137). Labels and option text should read correctly for whatever
-- journey_stage / purpose / visa_type / moving_with combination the user
-- answered — not just the pre-move planning case they were written for.
--
-- LABEL FIXES by broken case:
--   * Presumes the move is still ahead (broken for relocated/settled):
--     destination_country ("relocating to"), origin_country ("relocating
--     from" + "current country" placeholder — reintroduces the 001107 data
--     bug in the hint text), visa_type ("applies to your move"),
--     moving_with ("is making"), dependents_plan / pets_relocation /
--     spouse_career ("you're moving with"), banking_setup ("you'll bank"),
--     driving_transport ("you'll drive"), business_readiness ("you're
--     relocating"), study pair ("you're going the study route").
--   * Presumes a decision was made (broken for considering):
--     purpose ("Why are you relocating?" → reason for the move),
--     travel_date ("planning" → plan — or hope — to).
--   * Label/option mismatch: emotional_readiness asked about "feeling" but
--     its options score readiness — now asks readiness directly.
--   * Truncated phrasing: language_test ("where are you?" → "with it").
--   * Departure≠home: exit_plan ("exit from home" → "departure" — origin
--     may be a third country, not "home").
--   * Study-leaning examples: documents_ready gains "as required" so
--     non-study users don't read transcripts as mandatory.
--
-- OPTION FIXES (values unchanged — stored answers stay valid):
--   * Tense-bound escapes → "part of your move"-style neutral: moving_with
--     "moving alone"→"Just me", dependents_plan "Moving alone"→"No
--     dependants — not applicable", pets_relocation, spouse_career,
--     business_readiness ("Not relocating for business"→"Not a business
--     move"), driving_transport ("Won't be driving"→"I don't drive"),
--     banking_setup ("sorted before departure"→"Banking fully sorted").
--   * proof_of_funds gains the not_required escape its Band-A siblings
--     (language_test/visa_medical/police_clearance) already have — a
--     tourist/transit-route user previously had no honest answer. Scored 90,
--     consistent with the other not_required escapes.
--
-- POSITION FIX: 001105 inserted moving_with at position 7 where savings
-- already sat — a latent collision leaving render order between them
-- arbitrary. Shifting every field at position ≥ 7 (except moving_with)
-- right by one makes the context block contiguous at 1–7 and the scored
-- block at 8–44. Fresh installs reproduce the same collision (000735
-- inserts moving_with at 7 before 000896 renumbers savings to 7), so the
-- shift converges both paths identically.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

-- Position collision fix ---------------------------------------------------

UPDATE custom_fields SET position = position + 1
WHERE group_id = '00000000-0000-0000-0000-00000000a001'
  AND position >= 7
  AND key <> 'moving_with';

-- Context-field labels ------------------------------------------------------

UPDATE custom_fields SET label = 'Which country is your destination?'
WHERE id = '00000000-0000-0000-0000-00000000b001'; -- destination_country

UPDATE custom_fields SET label = 'Which country are you coming from?',
  placeholder = 'Select your origin country'
WHERE id = '00000000-0000-0000-0000-00000000b002'; -- origin_country

UPDATE custom_fields SET label = 'What is your visa route?'
WHERE id = '00000000-0000-0000-0000-00000000b003'; -- visa_type

UPDATE custom_fields SET label = 'What is the main reason for your move?'
WHERE id = '00000000-0000-0000-0000-00000000b005'; -- purpose

UPDATE custom_fields SET label = 'When do you plan — or hope — to travel or move?'
WHERE id = '00000000-0000-0000-0000-00000000b006'; -- travel_date

UPDATE custom_fields SET label = 'Who is part of this move? Select all that apply.',
  options = '[{"label": "Just me", "value": "alone"}, {"label": "Spouse or partner", "value": "partner"}, {"label": "Children or other dependants", "value": "dependents"}, {"label": "Pets", "value": "pets"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000b007'; -- moving_with

-- Scored-field labels -------------------------------------------------------

UPDATE custom_fields SET label = 'Are your key documents valid and ready (passport 6+ months validity, certificates, transcripts as required)?'
WHERE id = '00000000-0000-0000-0000-00000000c003'; -- documents_ready

UPDATE custom_fields SET label = 'If family or dependants are part of your move, are their arrangements (visas, school, care) sorted?',
  options = '[{"label": "No dependants — not applicable", "value": "alone"}, {"label": "Dependants coming, still planning", "value": "planning"}, {"label": "Partially arranged", "value": "partial"}, {"label": "Fully arranged", "value": "arranged"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000c009'; -- dependents_plan

UPDATE custom_fields SET label = 'If your visa or profession requires a language test (IELTS, TEF, Goethe, etc.), where are you with it?'
WHERE id = '00000000-0000-0000-0000-00000000d011'; -- language_test

UPDATE custom_fields SET label = 'If your route is study, how is your tuition & living-cost funding secured?'
WHERE id = '00000000-0000-0000-0000-00000000e014'; -- study_funding

UPDATE custom_fields SET label = 'If your route is study, where are you with admission (application, offer, deposit/enrolment)?'
WHERE id = '00000000-0000-0000-0000-00000000e016'; -- admission_status

UPDATE custom_fields SET label = 'Have you planned your departure (job notice, lease, subscriptions, belongings)?'
WHERE id = '00000000-0000-0000-0000-00000000e019'; -- exit_plan

UPDATE custom_fields SET label = 'Have you sorted how to bank and access money at your destination?',
  options = '[{"label": "No plan yet", "value": "none"}, {"label": "Researching options", "value": "researching"}, {"label": "Account opened or fintech set up", "value": "partial"}, {"label": "Banking fully sorted", "value": "sorted"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000e020'; -- banking_setup

UPDATE custom_fields SET label = 'Do you send — or plan to send — regular support to family or obligations back home? Is a remittance plan in place?'
WHERE id = '00000000-0000-0000-0000-00000000f022'; -- remittance_commitments

UPDATE custom_fields SET label = 'If pets are part of your move, are their vaccinations, permits, and transport arranged?',
  options = '[{"label": "No pets — not applicable", "value": "no_pets"}, {"label": "Pets coming, not started", "value": "not_started"}, {"label": "Vaccinations/permits in progress", "value": "arranging"}, {"label": "Fully arranged", "value": "sorted"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f024'; -- pets_relocation

UPDATE custom_fields SET label = 'If you drive — or plan to drive — at your destination, is your licence sorted (validity, international permit, conversion rules)?',
  options = '[{"label": "I don''t drive — not applicable", "value": "not_driving"}, {"label": "Haven''t checked the rules", "value": "unsure"}, {"label": "IDP/conversion in progress", "value": "in_progress"}, {"label": "Licence valid or conversion confirmed", "value": "sorted"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f026'; -- driving_transport

UPDATE custom_fields SET label = 'If your move is for business, how far along is your plan (market research, registration, funding)?',
  options = '[{"label": "Not a business move — not applicable", "value": "not_business"}, {"label": "Idea stage only", "value": "idea"}, {"label": "Plan developing / registration underway", "value": "developing"}, {"label": "Registered or revenue-ready", "value": "validated"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f027'; -- business_readiness

UPDATE custom_fields SET label = 'If a partner or spouse is part of your move, do they have a work or career plan at the destination?',
  options = '[{"label": "No partner — not applicable", "value": "no_partner"}, {"label": "No plan yet", "value": "not_started"}, {"label": "Planning or searching", "value": "planning"}, {"label": "Plan or job secured", "value": "secured"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f029'; -- spouse_career

UPDATE custom_fields SET label = 'How emotionally ready are you for this move?'
WHERE id = '00000000-0000-0000-0000-00000000a041'; -- emotional_readiness

-- proof_of_funds: add the missing not_required escape (scored 90 like the
-- other Band-A route-requirement escapes).
UPDATE custom_fields
SET options = options || '{"label": "Not required for my route", "value": "not_required"}'::jsonb,
    config = jsonb_set(config, '{option_scores,not_required}', '90')
WHERE id = '00000000-0000-0000-0000-00000000c001' -- proof_of_funds
  AND NOT options @> '[{"value": "not_required"}]'::jsonb;

COMMIT;
