-- 001108_readiness_stage_first.down.sql
-- Restore the pre-001108 context order and ungate purpose.

BEGIN;

UPDATE custom_fields SET position = 1
WHERE group_id = '00000000-0000-0000-0000-00000000a001' AND key = 'destination_country';
UPDATE custom_fields SET position = 2
WHERE group_id = '00000000-0000-0000-0000-00000000a001' AND key = 'origin_country';
UPDATE custom_fields SET position = 3
WHERE group_id = '00000000-0000-0000-0000-00000000a001' AND key = 'journey_stage';

UPDATE custom_fields SET config = config - 'visible_if'
WHERE id = '00000000-0000-0000-0000-00000000b005'; -- purpose

COMMIT;
