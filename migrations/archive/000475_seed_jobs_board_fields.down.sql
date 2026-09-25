-- 000475_seed_jobs_board_fields.down.sql
-- Reverse the Jobs Board field group + custom fields + application form seed.
-- Ref: japaconnect-engineering/sections/63-jobs-board.md

BEGIN;

DELETE FROM custom_field_values
WHERE field_id IN (
    SELECT id FROM custom_fields
    WHERE group_id = '00000000-0000-0000-0000-0000000000a0'
);

DELETE FROM custom_fields
WHERE group_id = '00000000-0000-0000-0000-0000000000a0';

DELETE FROM custom_field_group_assignments
WHERE group_id = '00000000-0000-0000-0000-0000000000a0';

DELETE FROM custom_field_groups
WHERE id = '00000000-0000-0000-0000-0000000000a0';

DELETE FROM forms
WHERE id = '00000000-0000-0000-0000-0000000000b0';

COMMIT;
