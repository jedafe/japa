-- 000896_readiness_expanded_questions.down.sql
-- Reverts the expanded readiness questions: removes the new fields, restores
-- the original positions, dimension weights, and description.

BEGIN;

DO $$ DECLARE
    gid uuid := '00000000-0000-0000-0000-00000000a001';
BEGIN

DELETE FROM custom_fields
WHERE group_id = gid AND id IN (
    '00000000-0000-0000-0000-00000000c001',
    '00000000-0000-0000-0000-00000000c002',
    '00000000-0000-0000-0000-00000000c003',
    '00000000-0000-0000-0000-00000000c004',
    '00000000-0000-0000-0000-00000000c005',
    '00000000-0000-0000-0000-00000000c006',
    '00000000-0000-0000-0000-00000000c007',
    '00000000-0000-0000-0000-00000000c008',
    '00000000-0000-0000-0000-00000000c009',
    '00000000-0000-0000-0000-00000000c010',
    '00000000-0000-0000-0000-00000000d011',
    '00000000-0000-0000-0000-00000000d012',
    '00000000-0000-0000-0000-00000000d013'
);

UPDATE custom_fields SET position = 1  WHERE group_id = gid AND key = 'savings';
UPDATE custom_fields SET position = 2  WHERE group_id = gid AND key = 'income_source';
UPDATE custom_fields SET position = 3  WHERE group_id = gid AND key = 'visa_stage';
UPDATE custom_fields SET position = 4  WHERE group_id = gid AND key = 'accommodation';
UPDATE custom_fields SET position = 5  WHERE group_id = gid AND key = 'skill_demand';
UPDATE custom_fields SET position = 6  WHERE group_id = gid AND key = 'emotional_readiness';
UPDATE custom_fields SET position = 7  WHERE group_id = gid AND key = 'destination_country';
UPDATE custom_fields SET position = 8  WHERE group_id = gid AND key = 'origin_country';
UPDATE custom_fields SET position = 9  WHERE group_id = gid AND key = 'visa_type';
UPDATE custom_fields SET position = 10 WHERE group_id = gid AND key = 'journey_stage';
UPDATE custom_fields SET position = 11 WHERE group_id = gid AND key = 'purpose';
UPDATE custom_fields SET position = 12 WHERE group_id = gid AND key = 'travel_date';

UPDATE forms
SET scoring_config = jsonb_set(
    scoring_config,
    '{dimension_weights}',
    '{"financial": 0.35, "logistical": 0.30, "professional": 0.20, "personal": 0.15}'::jsonb,
    true
)
WHERE id = '00000000-0000-0000-0000-00000000a002'
  AND form_type = 'assessment';

UPDATE forms
SET description = 'A scored questionnaire across financial, logistical, professional, and personal dimensions. Get a readiness score, threshold label, and a personalized roadmap.'
WHERE id = '00000000-0000-0000-0000-00000000a002'
  AND form_type = 'assessment';

END $$;

COMMIT;
