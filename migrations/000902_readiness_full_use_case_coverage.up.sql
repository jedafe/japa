-- 000902_readiness_full_use_case_coverage.up.sql
-- Final expansion of the default Japa Readiness Assessment (§71, ADR-071,
-- follow-up to 000896/000897) so the scored questionnaire covers the
-- remaining relocation use cases that still had no scored question:
--   * tax & legal obligations (exit filings, treaties, tax residency)
--   * ongoing remittance / family-support commitments back home
--   * shipping / selling / storing belongings
--   * relocating with pets (vaccinations, permits, transport)
--   * continuity of ongoing medical care and prescriptions
--   * driving licence validity / IDP / conversion at destination
--   * business-route preparedness (registration, market research, funding)
--   * professional network building at destination
--   * partner/spouse career plan
--   * contingency / fallback plan
--   * mentor or expat connection (feeds §102 mentorship)
--
-- All new questions map to existing scored dimensions, so
-- dimension_weights and thresholds need no change — scores normalise
-- within each dimension. Options that mean "this does not apply to my
-- route" score high so non-applicable answers never penalise the user.
-- Idempotent via deterministic UUIDs + ON CONFLICT (id) DO NOTHING.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

DO $$ DECLARE
    gid uuid := '00000000-0000-0000-0000-00000000a001';
BEGIN

INSERT INTO custom_fields (id, group_id, label, key, field_type, options, placeholder, is_required, is_system, show_in_frontend, position, config) VALUES

-- Financial
('00000000-0000-0000-0000-00000000f021', gid,
 'Have you sorted your tax and legal obligations (exit filings, dual-taxation treaties, tax residency status)?',
 'tax_obligations', 'select',
 '[{"value":"unaware","label":"Haven''t looked into it"},{"value":"not_started","label":"Aware but not started"},{"value":"in_progress","label":"Working through the requirements"},{"value":"sorted","label":"Fully sorted / not applicable"}]'::jsonb,
 'Select the closest match', true, false, true, 33,
 '{"weight":1.0,"dimension":"financial","option_scores":{"unaware":10,"not_started":25,"in_progress":60,"sorted":100}}'::jsonb),

('00000000-0000-0000-0000-00000000f022', gid,
 'Will you regularly support family or obligations back home? Is a remittance plan in place?',
 'remittance_commitments', 'select',
 '[{"value":"no_obligation","label":"No regular obligations"},{"value":"unplanned","label":"Obligations exist, no plan yet"},{"value":"planned","label":"Planned (providers compared, budget set)"},{"value":"setup","label":"Remittance channel set up and tested"}]'::jsonb,
 'Select the closest match', true, false, true, 34,
 '{"weight":1.0,"dimension":"financial","option_scores":{"no_obligation":90,"unplanned":30,"planned":65,"setup":100}}'::jsonb),

-- Logistical
('00000000-0000-0000-0000-00000000f023', gid,
 'How are your belongings getting there (shipping, selling, or storage)?',
 'belongings_shipping', 'select',
 '[{"value":"minimal","label":"Travelling light — nothing to ship"},{"value":"undecided","label":"Haven''t decided yet"},{"value":"planning","label":"Comparing movers / selling items"},{"value":"arranged","label":"Shipping, sale, or storage booked"}]'::jsonb,
 'Select the closest match', true, false, true, 35,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"minimal":85,"undecided":20,"planning":60,"arranged":100}}'::jsonb),

('00000000-0000-0000-0000-00000000f024', gid,
 'If you''re moving with pets, are their vaccinations, permits, and transport arranged?',
 'pets_relocation', 'select',
 '[{"value":"no_pets","label":"Not moving with pets"},{"value":"not_started","label":"Pets coming, not started"},{"value":"arranging","label":"Vaccinations/permits in progress"},{"value":"sorted","label":"Fully arranged"}]'::jsonb,
 'Select the closest match', true, false, true, 36,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"no_pets":90,"not_started":15,"arranging":60,"sorted":100}}'::jsonb),

('00000000-0000-0000-0000-00000000f025', gid,
 'Do you have ongoing medical needs (prescriptions, treatment, specialist care) arranged for continuity at your destination?',
 'healthcare_continuity', 'select',
 '[{"value":"none_needed","label":"No ongoing medical needs"},{"value":"not_planned","label":"Needs exist, no plan yet"},{"value":"researching","label":"Researching providers and prescription transfers"},{"value":"arranged","label":"Continuity of care arranged"}]'::jsonb,
 'Select the closest match', true, false, true, 37,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"none_needed":90,"not_planned":20,"researching":55,"arranged":100}}'::jsonb),

('00000000-0000-0000-0000-00000000f026', gid,
 'If you''ll drive at your destination, is your licence sorted (validity, international permit, conversion rules)?',
 'driving_transport', 'select',
 '[{"value":"not_driving","label":"Won''t be driving"},{"value":"unsure","label":"Haven''t checked the rules"},{"value":"in_progress","label":"IDP/conversion in progress"},{"value":"sorted","label":"Licence valid or conversion confirmed"}]'::jsonb,
 'Select the closest match', true, false, true, 38,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"not_driving":90,"unsure":35,"in_progress":65,"sorted":100}}'::jsonb),

-- Professional
('00000000-0000-0000-0000-00000000f027', gid,
 'If you''re relocating for business, how far along is your plan (market research, registration, funding)?',
 'business_readiness', 'select',
 '[{"value":"not_business","label":"Not relocating for business"},{"value":"idea","label":"Idea stage only"},{"value":"developing","label":"Plan developing / registration underway"},{"value":"validated","label":"Registered or revenue-ready"}]'::jsonb,
 'Select the closest match', true, false, true, 39,
 '{"weight":1.0,"dimension":"professional","option_scores":{"not_business":90,"idea":25,"developing":60,"validated":100}}'::jsonb),

('00000000-0000-0000-0000-00000000f028', gid,
 'Have you built professional connections at your destination (recruiters, associations, online network)?',
 'professional_network', 'select',
 '[{"value":"none","label":"No connections yet"},{"value":"few","label":"A few contacts"},{"value":"active","label":"Active network of contacts"},{"value":"established","label":"Established network with opportunities in motion"}]'::jsonb,
 'Select the closest match', true, false, true, 40,
 '{"weight":1.0,"dimension":"professional","option_scores":{"none":15,"few":45,"active":75,"established":100}}'::jsonb),

-- Personal
('00000000-0000-0000-0000-00000000f029', gid,
 'If you''re moving with a partner or spouse, do they have a work or career plan at the destination?',
 'spouse_career', 'select',
 '[{"value":"no_partner","label":"Not moving with a partner"},{"value":"not_started","label":"No plan yet"},{"value":"planning","label":"Planning or searching"},{"value":"secured","label":"Plan or job secured"}]'::jsonb,
 'Select the closest match', true, false, true, 41,
 '{"weight":1.0,"dimension":"personal","option_scores":{"no_partner":90,"not_started":25,"planning":60,"secured":100}}'::jsonb),

('00000000-0000-0000-0000-00000000f030', gid,
 'Do you have a fallback plan if the move doesn''t work out (return plan, backup finances, alternative route)?',
 'contingency_plan', 'select',
 '[{"value":"none","label":"No fallback plan"},{"value":"vague","label":"A vague idea"},{"value":"outline","label":"An outlined plan B"},{"value":"solid","label":"A solid, actionable fallback"}]'::jsonb,
 'Select the closest match', true, false, true, 42,
 '{"weight":1.0,"dimension":"personal","option_scores":{"none":15,"vague":40,"outline":65,"solid":100}}'::jsonb),

('00000000-0000-0000-0000-00000000f031', gid,
 'Have you connected with someone who''s already made this move (mentor, expat, friend)?',
 'mentor_connection', 'select',
 '[{"value":"none","label":"No one yet"},{"value":"looking","label":"Looking for one"},{"value":"some_contact","label":"In contact with expats"},{"value":"mentor","label":"Have a mentor or close guide"}]'::jsonb,
 'Select the closest match', true, false, true, 43,
 '{"weight":1.0,"dimension":"personal","option_scores":{"none":25,"looking":50,"some_contact":75,"mentor":100}}'::jsonb)

ON CONFLICT (id) DO NOTHING;

END $$;

COMMIT;
