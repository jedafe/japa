-- 001109_readiness_stage_neutral_labels.down.sql
-- Restore the pre-001109 labels/options and the shared position-7 slot.

BEGIN;

UPDATE custom_fields SET position = position - 1
WHERE group_id = '00000000-0000-0000-0000-00000000a001'
  AND position >= 8
  AND key <> 'moving_with';

UPDATE custom_fields SET label = 'Which country are you relocating to?'
WHERE id = '00000000-0000-0000-0000-00000000b001';

UPDATE custom_fields SET label = 'Which country are you relocating from?',
  placeholder = 'Select your current country'
WHERE id = '00000000-0000-0000-0000-00000000b002';

UPDATE custom_fields SET label = 'Which visa route applies to your move?'
WHERE id = '00000000-0000-0000-0000-00000000b003';

UPDATE custom_fields SET label = 'Why are you relocating?'
WHERE id = '00000000-0000-0000-0000-00000000b005';

UPDATE custom_fields SET label = 'When are you planning to travel or move?'
WHERE id = '00000000-0000-0000-0000-00000000b006';

UPDATE custom_fields SET label = 'Who is making this move with you? Select all that apply.',
  options = '[{"label": "Just me — moving alone", "value": "alone"}, {"label": "Spouse or partner", "value": "partner"}, {"label": "Children or other dependants", "value": "dependents"}, {"label": "Pets", "value": "pets"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000b007';

UPDATE custom_fields SET label = 'Are your key documents valid and ready (passport 6+ months validity, certificates, transcripts)?'
WHERE id = '00000000-0000-0000-0000-00000000c003';

UPDATE custom_fields SET label = 'If you''re moving with family or dependants, are their arrangements (visas, school, care) sorted?',
  options = '[{"label": "Moving alone", "value": "alone"}, {"label": "Dependants coming, still planning", "value": "planning"}, {"label": "Partially arranged", "value": "partial"}, {"label": "Fully arranged", "value": "arranged"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000c009';

UPDATE custom_fields SET label = 'If your visa or profession requires a language test (IELTS, TEF, Goethe, etc.), where are you?'
WHERE id = '00000000-0000-0000-0000-00000000d011';

UPDATE custom_fields SET label = 'If you''re going the study route, how is your tuition & living-cost funding secured?'
WHERE id = '00000000-0000-0000-0000-00000000e014';

UPDATE custom_fields SET label = 'If you''re going the study route, where are you with admission (application, offer, deposit/enrolment)?'
WHERE id = '00000000-0000-0000-0000-00000000e016';

UPDATE custom_fields SET label = 'Have you planned your exit from home (job notice, lease, subscriptions, belongings)?'
WHERE id = '00000000-0000-0000-0000-00000000e019';

UPDATE custom_fields SET label = 'Have you sorted how you''ll bank and access money at your destination?',
  options = '[{"label": "No plan yet", "value": "none"}, {"label": "Researching options", "value": "researching"}, {"label": "Account opened or fintech set up", "value": "partial"}, {"label": "Banking sorted before departure", "value": "sorted"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000e020';

UPDATE custom_fields SET label = 'Will you regularly support family or obligations back home? Is a remittance plan in place?'
WHERE id = '00000000-0000-0000-0000-00000000f022';

UPDATE custom_fields SET label = 'If you''re moving with pets, are their vaccinations, permits, and transport arranged?',
  options = '[{"label": "Not moving with pets", "value": "no_pets"}, {"label": "Pets coming, not started", "value": "not_started"}, {"label": "Vaccinations/permits in progress", "value": "arranging"}, {"label": "Fully arranged", "value": "sorted"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f024';

UPDATE custom_fields SET label = 'If you''ll drive at your destination, is your licence sorted (validity, international permit, conversion rules)?',
  options = '[{"label": "Won''t be driving", "value": "not_driving"}, {"label": "Haven''t checked the rules", "value": "unsure"}, {"label": "IDP/conversion in progress", "value": "in_progress"}, {"label": "Licence valid or conversion confirmed", "value": "sorted"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f026';

UPDATE custom_fields SET label = 'If you''re relocating for business, how far along is your plan (market research, registration, funding)?',
  options = '[{"label": "Not relocating for business", "value": "not_business"}, {"label": "Idea stage only", "value": "idea"}, {"label": "Plan developing / registration underway", "value": "developing"}, {"label": "Registered or revenue-ready", "value": "validated"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f027';

UPDATE custom_fields SET label = 'If you''re moving with a partner or spouse, do they have a work or career plan at the destination?',
  options = '[{"label": "Not moving with a partner", "value": "no_partner"}, {"label": "No plan yet", "value": "not_started"}, {"label": "Planning or searching", "value": "planning"}, {"label": "Plan or job secured", "value": "secured"}]'::jsonb
WHERE id = '00000000-0000-0000-0000-00000000f029';

UPDATE custom_fields SET label = 'How are you feeling about the move emotionally?'
WHERE id = '00000000-0000-0000-0000-00000000a041';

UPDATE custom_fields
SET options = (
        SELECT COALESCE(jsonb_agg(el), '[]'::jsonb)
        FROM jsonb_array_elements(options) el
        WHERE el->>'value' <> 'not_required'
    ),
    config = config #- '{option_scores,not_required}'
WHERE id = '00000000-0000-0000-0000-00000000c001'; -- proof_of_funds

COMMIT;
