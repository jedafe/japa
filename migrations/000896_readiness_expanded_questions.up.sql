-- 000896_readiness_expanded_questions.up.sql
-- Expand the default Japa Readiness Assessment (§71, ADR-071) so the scored
-- questionnaire covers the full relocation use-case surface: proof of funds,
-- relocation budget, documents, health insurance, travel logistics,
-- credential evaluation, job-search progress, support network, dependants,
-- language proficiency/test, destination research, and a 90-day settlement
-- plan. Adds two new scored dimensions — language and destination_knowledge —
-- which are already covered by the built-in curated roadmap library.
--
-- The six non-scored journey-context fields move to positions 1-6 so the
-- destination selector is the first UI input (geo-aware tool pattern). All
-- changes are idempotent via deterministic UUIDs and plain UPDATEs.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

DO $$ DECLARE
    gid uuid := '00000000-0000-0000-0000-00000000a001';
BEGIN

-- 1. Reorder: journey-context fields first (destination selector = first
--    input), then scored questions grouped by dimension.
UPDATE custom_fields SET position = 1  WHERE group_id = gid AND key = 'destination_country';
UPDATE custom_fields SET position = 2  WHERE group_id = gid AND key = 'origin_country';
UPDATE custom_fields SET position = 3  WHERE group_id = gid AND key = 'visa_type';
UPDATE custom_fields SET position = 4  WHERE group_id = gid AND key = 'journey_stage';
UPDATE custom_fields SET position = 5  WHERE group_id = gid AND key = 'purpose';
UPDATE custom_fields SET position = 6  WHERE group_id = gid AND key = 'travel_date';
UPDATE custom_fields SET position = 7  WHERE group_id = gid AND key = 'savings';
UPDATE custom_fields SET position = 8  WHERE group_id = gid AND key = 'income_source';
UPDATE custom_fields SET position = 11 WHERE group_id = gid AND key = 'visa_stage';
UPDATE custom_fields SET position = 15 WHERE group_id = gid AND key = 'accommodation';
UPDATE custom_fields SET position = 16 WHERE group_id = gid AND key = 'skill_demand';
UPDATE custom_fields SET position = 19 WHERE group_id = gid AND key = 'emotional_readiness';

-- 2. New scored questions. Same contract as the original seed: options[]
--    drives the UI control; config.option_scores keys must match the option
--    values; weight scales the field within its dimension.
INSERT INTO custom_fields (id, group_id, label, key, field_type, options, placeholder, is_required, is_system, show_in_frontend, position, config) VALUES

-- Financial
('00000000-0000-0000-0000-00000000c001', gid,
 'Do you meet the proof-of-funds requirement for your visa route?',
 'proof_of_funds', 'select',
 '[{"value":"unaware","label":"I don''t know the requirement"},{"value":"below","label":"Below the required amount"},{"value":"close","label":"Close to the required amount"},{"value":"met","label":"Requirement fully met"}]'::jsonb,
 'Select the closest match', true, false, true, 9,
 '{"weight":1.0,"dimension":"financial","option_scores":{"unaware":10,"below":30,"close":60,"met":100}}'::jsonb),

('00000000-0000-0000-0000-00000000c002', gid,
 'Have you budgeted the full cost of your move (flights, shipping, first months'' costs)?',
 'relocation_budget', 'select',
 '[{"value":"none","label":"No budget yet"},{"value":"rough","label":"Rough estimate only"},{"value":"itemized","label":"Itemized budget drafted"},{"value":"funded","label":"Budget drafted and fully funded"}]'::jsonb,
 'Select the closest match', true, false, true, 10,
 '{"weight":1.0,"dimension":"financial","option_scores":{"none":10,"rough":40,"itemized":70,"funded":100}}'::jsonb),

-- Logistical
('00000000-0000-0000-0000-00000000c003', gid,
 'Are your key documents valid and ready (passport 6+ months validity, certificates, transcripts)?',
 'documents_ready', 'select',
 '[{"value":"not_started","label":"Not started gathering them"},{"value":"gathering","label":"Gathering documents"},{"value":"mostly","label":"Mostly ready, some expiring or missing"},{"value":"ready","label":"All valid and certified/attested"}]'::jsonb,
 'Select the closest match', true, false, true, 12,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"not_started":10,"gathering":30,"mostly":70,"ready":100}}'::jsonb),

('00000000-0000-0000-0000-00000000c004', gid,
 'Is health insurance or healthcare coverage arranged for your arrival?',
 'health_insurance', 'select',
 '[{"value":"none","label":"No plan yet"},{"value":"researching","label":"Researching options"},{"value":"short_term","label":"Short-term cover arranged"},{"value":"covered","label":"Fully covered from day one"}]'::jsonb,
 'Select the closest match', true, false, true, 13,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"none":10,"researching":40,"short_term":70,"covered":100}}'::jsonb),

('00000000-0000-0000-0000-00000000c005', gid,
 'How far along are your travel logistics (flights, luggage, first-night stay)?',
 'travel_logistics', 'select',
 '[{"value":"not_started","label":"Not started"},{"value":"researching","label":"Comparing options"},{"value":"partial","label":"Partially booked"},{"value":"booked","label":"Fully booked"}]'::jsonb,
 'Select the closest match', true, false, true, 14,
 '{"weight":1.0,"dimension":"logistical","option_scores":{"not_started":10,"researching":30,"partial":70,"booked":100}}'::jsonb),

-- Professional
('00000000-0000-0000-0000-00000000c006', gid,
 'Have your qualifications been assessed for recognition at your destination (e.g. WES/ECA, licensing body)?',
 'credential_evaluation', 'select',
 '[{"value":"unaware","label":"Haven''t looked into it"},{"value":"required_not_started","label":"Required, not started"},{"value":"in_progress","label":"Evaluation in progress"},{"value":"done","label":"Completed / not required"}]'::jsonb,
 'Select the closest match', true, false, true, 17,
 '{"weight":1.0,"dimension":"professional","option_scores":{"unaware":10,"required_not_started":20,"in_progress":60,"done":100}}'::jsonb),

('00000000-0000-0000-0000-00000000c007', gid,
 'How far along is your job search in your destination?',
 'job_search_progress', 'select',
 '[{"value":"not_started","label":"Not started"},{"value":"cv_ready","label":"CV/resume localized for the market"},{"value":"applying","label":"Applying or interviewing"},{"value":"offer","label":"Offer secured"}]'::jsonb,
 'Select the closest match', true, false, true, 18,
 '{"weight":1.0,"dimension":"professional","option_scores":{"not_started":10,"cv_ready":40,"applying":70,"offer":100}}'::jsonb),

-- Personal
('00000000-0000-0000-0000-00000000c008', gid,
 'Do you have a support network at your destination (family, friends, community)?',
 'support_network', 'select',
 '[{"value":"none","label":"No one there yet"},{"value":"few","label":"A few contacts"},{"value":"community","label":"Active community connections"},{"value":"family","label":"Close family or friends already there"}]'::jsonb,
 'Select the closest match', true, false, true, 20,
 '{"weight":1.0,"dimension":"personal","option_scores":{"none":20,"few":40,"community":70,"family":100}}'::jsonb),

('00000000-0000-0000-0000-00000000c009', gid,
 'If you''re moving with family or dependants, are their arrangements (visas, school, care) sorted?',
 'dependents_plan', 'select',
 '[{"value":"alone","label":"Moving alone"},{"value":"planning","label":"Dependants coming, still planning"},{"value":"partial","label":"Partially arranged"},{"value":"arranged","label":"Fully arranged"}]'::jsonb,
 'Select the closest match', true, false, true, 21,
 '{"weight":1.0,"dimension":"personal","option_scores":{"alone":90,"planning":40,"partial":70,"arranged":100}}'::jsonb),

-- Language (new dimension — covered by the built-in curated roadmap library)
('00000000-0000-0000-0000-00000000c010', gid,
 'How proficient are you in the main language of your destination?',
 'language_proficiency', 'select',
 '[{"value":"none","label":"No proficiency"},{"value":"beginner","label":"Beginner"},{"value":"conversational","label":"Conversational"},{"value":"fluent","label":"Professional or native fluency"}]'::jsonb,
 'Select the closest match', true, false, true, 22,
 '{"weight":1.0,"dimension":"language","option_scores":{"none":10,"beginner":35,"conversational":70,"fluent":100}}'::jsonb),

('00000000-0000-0000-0000-00000000d011', gid,
 'If your visa or profession requires a language test (IELTS, TEF, Goethe, etc.), where are you?',
 'language_test', 'select',
 '[{"value":"not_required","label":"Not required for my route"},{"value":"not_started","label":"Required, not started"},{"value":"preparing","label":"Preparing / booked"},{"value":"passed","label":"Passed at required level"}]'::jsonb,
 'Select the closest match', true, false, true, 23,
 '{"weight":1.0,"dimension":"language","option_scores":{"not_required":90,"not_started":10,"preparing":50,"passed":100}}'::jsonb),

-- Destination knowledge (new dimension — covered by the curated library)
('00000000-0000-0000-0000-00000000d012', gid,
 'How well do you know your destination (cost of living, neighbourhoods, transport, safety, healthcare)?',
 'destination_research', 'select',
 '[{"value":"none","label":"Haven''t researched it"},{"value":"some","label":"Some online research"},{"value":"good","label":"Good working knowledge"},{"value":"deep","label":"Lived or visited extensively"}]'::jsonb,
 'Select the closest match', true, false, true, 24,
 '{"weight":1.0,"dimension":"destination_knowledge","option_scores":{"none":10,"some":40,"good":70,"deep":100}}'::jsonb),

('00000000-0000-0000-0000-00000000d013', gid,
 'Do you have a concrete 90-day settlement plan (bank account, phone, registration/ID, tax number)?',
 'settlement_plan', 'select',
 '[{"value":"none","label":"No plan yet"},{"value":"rough","label":"Rough idea"},{"value":"mostly","label":"Mostly planned"},{"value":"detailed","label":"Detailed week-by-week plan"}]'::jsonb,
 'Select the closest match', true, false, true, 25,
 '{"weight":1.0,"dimension":"destination_knowledge","option_scores":{"none":10,"rough":40,"mostly":70,"detailed":100}}'::jsonb)

ON CONFLICT (id) DO NOTHING;

-- 3. Rebalance dimension weights for the six scored dimensions. Only the
--    dimension_weights key is replaced — thresholds, max_score, and any
--    admin-configured curated_roadmap are preserved.
UPDATE forms
SET scoring_config = jsonb_set(
    scoring_config,
    '{dimension_weights}',
    '{
        "financial": 0.25,
        "logistical": 0.25,
        "professional": 0.20,
        "personal": 0.15,
        "language": 0.10,
        "destination_knowledge": 0.05
    }'::jsonb,
    true
)
WHERE id = '00000000-0000-0000-0000-00000000a002'
  AND form_type = 'assessment';

-- 4. Refresh the public description to match the expanded dimensions.
UPDATE forms
SET description = 'A scored questionnaire across financial, logistical, professional, language, destination-knowledge, and personal dimensions. Get a readiness score, threshold label, and a personalized roadmap.'
WHERE id = '00000000-0000-0000-0000-00000000a002'
  AND form_type = 'assessment';

END $$;

COMMIT;
