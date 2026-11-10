-- 001108_readiness_stage_first.down.sql
-- Restore the pre-001108 context order and ungate purpose.

BEGIN;

UPDATE custom_fields SET position = CASE key
  WHEN 'destination_country' THEN 1
  WHEN 'origin_country' THEN 2
  WHEN 'journey_stage' THEN 3
  WHEN 'purpose' THEN 4
  WHEN 'visa_type' THEN 5
  WHEN 'travel_date' THEN 6
END
WHERE group_id = '00000000-0000-0000-0000-00000000a001'
  AND key IN ('journey_stage', 'destination_country', 'origin_country',
              'purpose', 'visa_type', 'travel_date');

UPDATE custom_fields SET config = config - 'visible_if'
WHERE id = '00000000-0000-0000-0000-00000000b005'; -- purpose

COMMIT;
