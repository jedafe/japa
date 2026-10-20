-- 000897_readiness_additional_use_cases.up.sql
-- Extends the expanded Japa Readiness Assessment (§71, ADR-071, follow-up to
-- 000896) to cover the remaining relocation use cases that had no scored
-- question:
--   * study route — admission progress and tuition/scholarship funding
--   * mandatory medical exams (TB test, immigration medical exam)
--   * police clearance certificates
--   * contingency/emergency buffer beyond the relocation budget
--   * day-one money access (foreign account / fintech / forex plan)
--   * winding down home-country commitments (job notice, lease, belongings)
--
-- All new questions map to existing scored dimensions, so dimension_weights
-- and thresholds need no change — scores normalise within each dimension.
-- Options that mean "this does not apply to my route" score high so
-- non-applicable answers never penalise the user. Idempotent via
-- deterministic UUIDs + ON CONFLICT (id) DO NOTHING.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

DO $$ DECLARE
    gid uuid := '00000000-0000-0000-0000-00000000a001';
BEGIN

INSERT INTO custom_fields (id, group_id, label, key, field_type, options, placeholder, is_required, is_system, show_in_frontend, position, config) VALUES

-- Financial
('00000000-0000-0000-0000-00000000e014', gid,
 'If you''re going the study route, how is your tuition & living-cost funding secured?',
 'study_funding', 'select',
 '[{"value":"not_study","label":"Not studying — not applicable"},{"value":"unfunded","label":"No funding secured yet"},{"value":"partial","label":"Partially funded / awaiting scholarship decision"},{"value":"funded","label":"Fully funded (savings, scholarship, or sponsor)"}]'::jsonb,
 'Select the closest match', true, false, true, 26,
 '{"weight":1.0,"dimension":"financial","option_scores":{"not_study":90,"unfunded":15,"partial":60,"funded":100}}'::jsonb),

('00000000-0000-0000-0000-00000000e015', gid,
 'Beyond your relocation budget, do you have an emergency buffer for your first months?',
 'emergency_buffer', 'select',
 '[{"value":"none","label":"No buffer"},{"value":"under_1","label":"Under 1 month of expenses"},{"value":"1_to_3","label":"1–3 months of expenses"},{"value":"over_3","label":"3+ months of expenses"}]'::jsonb,
 'Select the closest match', true, false, true, 27,
 '{"weight":1.0,"dimension":"financial","option_scores":{"none":10,"under_1":40,"1_to_3":70,"over_3":100}}'::jsonb),

-- Logistical
('00000000-0000-0000-0000-00000000e016', gid,
 'If you''re going the study route, where are you with admission (application, offer, deposit/enrolment)?',
 'admission_status', 'select',
 '[{"value":"not_study","label":"Not studying — not applicable"},{"value":"not_applied","label":"Haven''t applied yet"},{"value":"applied","label":"Applied, awaiting decision"},{"value":"offer","label":"Offer/admission secured (deposit paid or enrolled)"}]'::jsonb,
 'Select the closest match', true, false, true, 28,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"not_study":90,"not_applied":20,"applied":60,"offer":100}}'::jsonb),

('00000000-0000-0000-0000-00000000e017', gid,
 'Have you completed the medical exams your visa route requires (e.g. TB test, immigration medical exam)?',
 'visa_medical', 'select',
 '[{"value":"not_required","label":"Not required for my route"},{"value":"not_started","label":"Required, not started"},{"value":"booked","label":"Booked / scheduled"},{"value":"done","label":"Completed"}]'::jsonb,
 'Select the closest match', true, false, true, 29,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"not_required":90,"not_started":20,"booked":60,"done":100}}'::jsonb),

('00000000-0000-0000-0000-00000000e018', gid,
 'Do you have the police clearance certificates your visa route requires?',
 'police_clearance', 'select',
 '[{"value":"not_required","label":"Not required for my route"},{"value":"not_started","label":"Required, not started"},{"value":"requested","label":"Requested / processing"},{"value":"have","label":"Certificates in hand"}]'::jsonb,
 'Select the closest match', true, false, true, 30,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"not_required":90,"not_started":20,"requested":60,"have":100}}'::jsonb),

('00000000-0000-0000-0000-00000000e019', gid,
 'Have you planned your exit from home (job notice, lease, subscriptions, belongings)?',
 'exit_plan', 'select',
 '[{"value":"not_started","label":"Not thought about it yet"},{"value":"thinking","label":"Thinking it through"},{"value":"partial","label":"Partially planned"},{"value":"planned","label":"Fully planned with dates"}]'::jsonb,
 'Select the closest match', true, false, true, 31,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"not_started":15,"thinking":40,"partial":70,"planned":100}}'::jsonb),

-- Destination knowledge / settlement readiness
('00000000-0000-0000-0000-00000000e020', gid,
 'Have you sorted how you''ll bank and access money on arrival (foreign account, fintech, forex plan)?',
 'banking_setup', 'select',
 '[{"value":"none","label":"No plan yet"},{"value":"researching","label":"Researching options"},{"value":"partial","label":"Account opened or fintech set up"},{"value":"sorted","label":"Banking sorted before departure"}]'::jsonb,
 'Select the closest match', true, false, true, 32,
 '{"weight":1.0,"dimension":"destination_knowledge","option_scores":{"none":15,"researching":40,"partial":70,"sorted":100}}'::jsonb)

ON CONFLICT (id) DO NOTHING;

END $$;

COMMIT;
