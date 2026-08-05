-- 000279_seed_default_assessment_form.up.sql
-- Seed a default Japa Readiness Assessment form (Section 71, ADR-071) for the
-- default tenant so the /readiness tool surface has content out of the box.
-- The form is built on the §06 custom-field infrastructure: a custom_field_group
-- holds the scored questions, a forms row (form_type='assessment') holds the
-- scoring_config, and a custom_field_group_assignment links them.
--
-- Idempotent: ON CONFLICT DO NOTHING on the deterministic UUIDs.
--
-- Ref: japaconnect-engineering/sections/71-readiness-assessment.md

BEGIN;

-- Default tenant.
DO $$ DECLARE tid uuid := '00000000-0000-0000-0000-000000000001'; BEGIN

-- 1. Custom field group for the assessment questions.
INSERT INTO custom_field_groups (id, tenant_id, name, slug, description, is_system)
VALUES (
    '00000000-0000-0000-0000-00000000a001',
    tid,
    'Japa Readiness Assessment Fields',
    'japa_readiness_assessment_fields',
    'Scored questions for the Japa Readiness Assessment (financial, logistical, professional, personal dimensions).',
    false
)
ON CONFLICT (id) DO NOTHING;

-- 2. The assessment form (form_type='assessment', status='active').
--    scoring_config: max_score + threshold bands + per-dimension weights.
INSERT INTO forms (
    id, tenant_id, name, slug, description,
    is_multi_step, submit_label, success_message, status, is_system,
    form_type, scoring_config
)
VALUES (
    '00000000-0000-0000-0000-00000000a002',
    tid,
    'Japa Readiness Assessment',
    'japa-readiness-assessment',
    'A scored questionnaire across financial, logistical, professional, and personal dimensions. Get a readiness score, threshold label, and a personalized roadmap.',
    true,
    'Submit & get score',
    'Your readiness score is being calculated. Hang tight!',
    'active',
    false,
    'assessment',
    '{
        "max_score": 100,
        "thresholds": [
            {"min": 0,  "max": 39,  "label": "Not Ready Yet",   "color": "#ef4444"},
            {"min": 40, "max": 59,  "label": "Getting There",   "color": "#f59e0b"},
            {"min": 60, "max": 74,  "label": "Almost Ready",    "color": "#3b82f6"},
            {"min": 75, "max": 100, "label": "Japa Ready",      "color": "#22c55e"}
        ],
        "dimension_weights": {
            "financial":    0.35,
            "logistical":   0.30,
            "professional": 0.20,
            "personal":     0.15
        }
    }'::jsonb
)
ON CONFLICT (id) DO NOTHING;

-- 3. Link the field group to the form (entity_class='form').
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES (
    '00000000-0000-0000-0000-00000000a001',
    'form',
    '00000000-0000-0000-0000-00000000a002',
    0
)
ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

-- 4. The scored questions (custom_fields). Each select field has:
--    options jsonb: [{value, label}]  (drives the UI control)
--    config  jsonb: {weight, dimension, option_scores: {value -> raw score}}
--    The option_scores keys MUST match the options[].value (the scoring engine
--    looks up fsc.OptionScores[answerString]).

-- 4a. Financial dimension
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, placeholder, is_required, show_in_frontend, position, config)
VALUES (
    '00000000-0000-0000-0000-00000000a011',
    '00000000-0000-0000-0000-00000000a001',
    tid,
    'How much have you saved for your move?',
    'savings',
    'select',
    '[{"value":"lt_6m","label":"Less than 6 months of expenses"},{"value":"6_12m","label":"6 to 12 months of expenses"},{"value":"1_2y","label":"1 to 2 years of expenses"},{"value":"gt_2y","label":"More than 2 years of expenses"}]'::jsonb,
    'Select the closest match',
    true,
    true,
    1,
    '{"weight":1.0,"dimension":"financial","option_scores":{"lt_6m":10,"6_12m":30,"1_2y":60,"gt_2y":100}}'::jsonb
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, placeholder, is_required, show_in_frontend, position, config)
VALUES (
    '00000000-0000-0000-0000-00000000a012',
    '00000000-0000-0000-0000-00000000a001',
    tid,
    'Do you have a stable income source lined up at your destination?',
    'income_source',
    'select',
    '[{"value":"none","label":"No income source"},{"value":"freelance","label":"Freelance / remote work"},{"value":"offer","label":"Job offer in hand"},{"value":"employed","label":"Already employed there"}]'::jsonb,
    'Select the closest match',
    true,
    true,
    2,
    '{"weight":1.0,"dimension":"financial","option_scores":{"none":10,"freelance":40,"offer":80,"employed":100}}'::jsonb
)
ON CONFLICT (id) DO NOTHING;

-- 4b. Logistical dimension
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, placeholder, is_required, show_in_frontend, position, config)
VALUES (
    '00000000-0000-0000-0000-00000000a021',
    '00000000-0000-0000-0000-00000000a001',
    tid,
    'How far along is your visa process?',
    'visa_stage',
    'select',
    '[{"value":"not_started","label":"Not started"},{"value":"researching","label":"Researching requirements"},{"value":"applying","label":"Application in progress"},{"value":"approved","label":"Visa approved"}]'::jsonb,
    'Select the closest match',
    true,
    true,
    3,
    '{"weight":1.0,"dimension":"logistical","option_scores":{"not_started":10,"researching":30,"applying":60,"approved":100}}'::jsonb
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, placeholder, is_required, show_in_frontend, position, config)
VALUES (
    '00000000-0000-0000-0000-00000000a022',
    '00000000-0000-0000-0000-00000000a001',
    tid,
    'Have you secured accommodation at your destination?',
    'accommodation',
    'select',
    '[{"value":"none","label":"Not yet"},{"value":"researching","label":"Researching neighbourhoods"},{"value":"short_term","label":"Short-term booked"},{"value":"long_term","label":"Long-term secured"}]'::jsonb,
    'Select the closest match',
    true,
    true,
    4,
    '{"weight":1.0,"dimension":"logistical","option_scores":{"none":10,"researching":30,"short_term":70,"long_term":100}}'::jsonb
)
ON CONFLICT (id) DO NOTHING;

-- 4c. Professional dimension
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, placeholder, is_required, show_in_frontend, position, config)
VALUES (
    '00000000-0000-0000-0000-00000000a031',
    '00000000-0000-0000-0000-00000000a001',
    tid,
    'How in-demand is your skill set in your destination country?',
    'skill_demand',
    'select',
    '[{"value":"low","label":"Low demand"},{"value":"moderate","label":"Moderate demand"},{"value":"high","label":"High demand"},{"value":"very_high","label":"Very high demand"}]'::jsonb,
    'Select the closest match',
    true,
    true,
    5,
    '{"weight":1.0,"dimension":"professional","option_scores":{"low":20,"moderate":50,"high":75,"very_high":100}}'::jsonb
)
ON CONFLICT (id) DO NOTHING;

-- 4d. Personal dimension
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, placeholder, is_required, show_in_frontend, position, config)
VALUES (
    '00000000-0000-0000-0000-00000000a041',
    '00000000-0000-0000-0000-00000000a001',
    tid,
    'How prepared do you feel emotionally for the move?',
    'emotional_readiness',
    'select',
    '[{"value":"not_ready","label":"Not ready"},{"value":"somewhat","label":"Somewhat ready"},{"value":"mostly","label":"Mostly ready"},{"value":"fully","label":"Fully ready"}]'::jsonb,
    'Select the closest match',
    true,
    true,
    6,
    '{"weight":1.0,"dimension":"personal","option_scores":{"not_ready":20,"somewhat":50,"mostly":75,"fully":100}}'::jsonb
)
ON CONFLICT (id) DO NOTHING;

END $$;

COMMIT;
